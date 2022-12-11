package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/logex.v1"
)

var DefaultClient = &http.Client{}

type TaskConfig struct {
	UserAgent  string
	MaxSpeed   int64
	Clean      bool
	Progress   bool
	ShowRealSp bool
	Headers    []string
	Proxy      []string
}

func (t *TaskConfig) init() {
}

type DnTask struct {
	*TaskConfig
	source *url.URL
	Meta   *Meta

	file     *os.File
	writeOp  chan *writeOp
	stopChan chan struct{}

	wg    sync.WaitGroup
	start time.Time
	sync.Mutex
	rateLimit *RateLimit

	downloadPerSecond int64

	l *Liner
}

func NewDnTaskAuto(url_, pwd string, bit uint, cfg *TaskConfig) (*DnTask, error) {
	_, err := os.Stat(url_)
	if !cfg.Clean && err == nil {
		if meta, _ := NewMetaFormFile(url_); meta != nil {
			logex.Info("downloading form", meta.Source)
			return NewDnTask(meta.Source, pwd, meta.BlkBit, cfg)
		}
	}

	return NewDnTask(url_, pwd, bit, cfg)
}

func NewDnTask(url_, pwd string, bit uint, cfg *TaskConfig) (*DnTask, error) {
	if url_ == "" {
		return nil, logex.NewError("url is empty")
	}
	if cfg == nil {
		cfg = new(TaskConfig)
	}
	cfg.init()

	source, err := url.Parse(url_)
	if err != nil {
		return nil, logex.Trace(err)
	}
	meta, err := NewMeta(pwd, url_, bit, cfg.Clean)
	if err != nil {
		return nil, logex.Trace(err)
	}

	dn := &DnTask{
		TaskConfig: cfg,
		rateLimit:  NewRateLimit(cfg.MaxSpeed),
		source:     source,
		Meta:       meta,
		writeOp:    make(chan *writeOp, 1<<3),
		stopChan:   make(chan struct{}),
		start:      time.Now(),
		l:          NewLiner(os.Stderr),
	}
	if cfg.Clean {
		os.Remove(dn.Meta.targetPath())
	}

	if err = dn.Meta.retrieveFromDisk(cfg.Proxy, cfg.UserAgent); err != nil {
		dn.Meta.Remove()
		return nil, logex.Trace(err)
	}

	if err = dn.Meta.Sync(); err != nil {
		return nil, logex.Trace(err)
	}

	if err = dn.openFile(); err != nil {
		return nil, logex.Trace(err)
	}

	go dn.ioloop()
	go dn.progress()
	return dn, nil
}

func (d *DnTask) openFile() error {
	f, err := os.OpenFile(d.Meta.targetPath(), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return logex.Trace(err)
	}
	d.file = f
	return nil
}

type writeOpReply struct {
	N   int
	Err error
}

type writeOp struct {
	Offset int64
	Buf    []byte
	Reply  chan *writeOpReply
}

func (d *DnTask) ioloop() {
	var w *writeOp
	for {
		select {
		case w = <-d.writeOp:
		case <-d.stopChan:
			return
		}
		n, err := d.file.WriteAt(w.Buf, w.Offset)
		if err != nil {
			d.file.Close()
			d.file = nil
			if err := d.openFile(); err != nil {
				panic(err)
			}
		}
		if d.MaxSpeed > 0 {
			d.rateLimit.Process(n)
		}
		w.Reply <- &writeOpReply{n, logex.Trace(err)}
	}
}

// [start, end)
func (d *DnTask) allocDnBlk(off int) (idx int, start, end int64) {
	d.Lock()
	defer d.Unlock()

	for i := off; i < len(d.Meta.Blocks); i++ {
		blk := d.Meta.Blocks[i]
		if blk == nil {
			blk = NewBlock()
			d.Meta.Blocks[i] = blk
		}
		if blk.State != STATE_INIT {
			continue
		}
		blk.State = STATE_PROCESS
		offset := int64(i << d.Meta.BlkBit)
		start = offset + int64(blk.Written)
		end = int64((i + 1) << d.Meta.BlkBit)
		if end > d.Meta.FileSize {
			end = d.Meta.FileSize
		}
		return i, start, end
	}
	return -1, -1, -1
}

func setRange(h http.Header, start, end int64) {
	h.Set(H_RANGE, fmt.Sprintf("bytes=%d-%d", start, end-1))
}

func (d *DnTask) checkWritten(written, start, end int64) error {
	w := end - start - 1
	if w > 0 && written != w {
		return logex.NewError("written not expected:", written, w)
	}
	return nil
}

// call after written, offset changed
func (d *DnTask) onWriteFunc(offset int64, written int) error {
	if !d.Meta.IsAccpetRange() {
		d.Meta.MarkFinishStream(int64(written))
		return nil
	}

	err := d.Meta.MarkFinishByN(offset, written, true)
	return logex.Trace(err)
}

func (d *DnTask) httpDn(client *http.Client, req *http.Request, op *writeOp, start, end int64) (int64, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, logex.Trace(err)
	}
	rc := NewReader(resp.Body)
	defer rc.Close()

	r := bufio.NewReader(rc)
	w := NewFileWriter(d, start, op, d.writeOp, d.onWriteFunc)
	written, err := io.CopyN(w, r, end-start)
	if err != nil {
		return written, logex.Trace(err)
	}
	if resp.ContentLength != written {
		logex.Errorf("ContentLength is not expected: got %v, want: %v, range:%v,%v", resp.ContentLength, written, start, end)
	}
	io.Copy(ioutil.Discard, rc)
	return written, nil
}

func (d *DnTask) proxyGet(client *http.Client, host string, idx int, op *writeOp, start, end int64) (int64, error) {
	proxy := proxyUrl(host, d.Meta.Source, start, end)
	req, err := http.NewRequest("GET", proxy, nil)
	if err != nil {
		return 0, logex.Trace(err)
	}
	req.Header.Set("User-Agent", d.UserAgent)
	return d.httpDn(client, req, op, start, end)
}

func (d *DnTask) httpGet(client *http.Client, idx int, op *writeOp, start, end int64) (int64, error) {
	req, err := http.NewRequest("GET", d.Meta.Source, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", d.UserAgent)
	for _, h := range d.Headers {
		if idx := strings.Index(h, ":"); idx > 0 {
			req.Header.Set(h[:idx], strings.TrimSpace(h[idx+1:]))
		}
	}

	if idx >= 0 {
		setRange(req.Header, start, end)
	} else {
		start = 0
	}
	return d.httpDn(client, req, op, start, end)
}

func (d *DnTask) download(t *DnType) {
	var (
		idx        int
		start, end int64
		err        error
		retry      int

		maxRetry = 3
		op       = new(writeOp)
	)
	op.Reply = make(chan *writeOpReply)

	if !d.Meta.IsAccpetRange() {
		_, err = d.httpGet(DefaultClient, -1, op, -1, -1)
		if err != nil {
			logex.Error(err)
		}
		return
	}

	for {
		idx, start, end = d.allocDnBlk(idx)
		if idx < 0 {
			break
		}
		if t.Proxy == "" {
			_, err = d.httpGet(DefaultClient, idx, op, start, end)
		} else {
			_, err = d.proxyGet(DefaultClient, t.Proxy, idx, op, start, end)
		}
		if err != nil {
			if retry > maxRetry && !logex.Equal(err, io.EOF) {
				logex.Error(err)
				return
			}
			retry++
			d.Meta.MarkInit(idx)
			continue
		}

		idx++
	}
}

type DnType struct {
	Proxy string
}

func NewDnType(host string) *DnType {
	return &DnType{host}
}

func (d *DnTask) Schedule(n int) {
	if false && !d.Meta.IsAccpetRange() {
		n = 1
		logex.Info("range is not acceptable, turn to single thread")
	}
	if n > len(d.Meta.Blocks) {
		logex.Info("remote file size is too small to use", n, "threads, decrease to", len(d.Meta.Blocks))
		n = len(d.Meta.Blocks)
	}

	types := make([]*DnType, len(d.Proxy)+1)
	for i := 0; i < len(types); i++ {
		if i == len(types)-1 {
			types[i] = NewDnType("")
		} else {
			types[i] = NewDnType(d.Proxy[i])
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		i := i
		go func() {
			d.download(types[i%len(types)])
			wg.Done()
		}()
	}
	wg.Wait()
}

func calUnit(u int64) string {
	units := []string{
		"B", "KB", "MB", "GB", "TB", "PB", "ZB",
	}
	idx := 0
	data := float64(u)
	for data > 10240 {
		idx++
		data /= 1024
	}
	if idx < 2 {
		return fmt.Sprintf("%d%s", int(data), units[idx])
	} else {
		return fmt.Sprintf("%.2f%s", data, units[idx])
	}

}

func (t *DnTask) Close() {
	close(t.stopChan)
	t.wg.Wait()
	t.Meta.Close()
	t.l.Finish()
}

func (t *DnTask) progress() {
	t.wg.Add(1)
	defer t.wg.Done()

	lastWritten := int64(0)
	fileSize := t.Meta.FileSize
	size := calUnit(fileSize)
	ticker := time.NewTicker(time.Second)
	stop := false

	var totalSp, totalN int64
	for !stop {
		select {
		case <-ticker.C:
		case <-t.stopChan:
			stop = true
		}
		written := atomic.LoadInt64(&t.Meta.written)
		if lastWritten > 0 {
			totalSp += written - lastWritten
		}
		atomic.StoreInt64(&t.downloadPerSecond, written-lastWritten)
		totalN += 1
		if t.MaxSpeed > 0 {
			t.rateLimit.Reset()
		}
		realDn := atomic.SwapInt64(&report, 0)

		extend := "\b"
		if t.ShowRealSp {
			extend += fmt.Sprintf(" RL:%v", calUnit(realDn))
		}

		if t.Progress {
			t.l.Print(fmt.Sprintf("[%v/%v(%v%%) DL:%v TIME:%v ETA:%v %v]",
				calUnit(written),
				size,
				calProgress(written, fileSize),
				calUnit(written-lastWritten),
				calTime(time.Now().Sub(t.start)),
				calTime(calRemainTime(fileSize-written, totalSp/totalN)),
				extend,
			))
		}
		lastWritten = written
	}
}

func calRemainTime(remain int64, speed int64) time.Duration {
	if speed == 0 {
		return time.Duration(0)
	}
	return time.Duration(remain/speed) * time.Second
}

func calProgress(a, b int64) int64 {
	if b == 0 {
		return -1
	}
	return int64(a * 100 / b)
}

func calTime(d time.Duration) string {
	return (time.Duration(d.Seconds()) * time.Second).String()
}

type Liner struct {
	buf *bytes.Buffer

	io.Writer
	last int
}

func NewLiner(w io.Writer) *Liner {
	l := &Liner{Writer: w}
	return l
}

func (l *Liner) Print(objs ...interface{}) {
	if l.buf == nil {
		l.buf = bytes.NewBuffer(nil)
	}
	last := l.last
	if last > 0 {
		l.buf.Write(bytes.Repeat([]byte("\b"), last+2))
		last = 0
	}
	for _, o := range objs {
		n, _ := l.buf.WriteString(fmt.Sprintf("%v", o))
		last += n
	}
	if last < l.last {
		l.buf.Write(bytes.Repeat([]byte(" "), l.last-last))
	} else {
		l.last = last
	}
	l.Writer.Write(l.buf.Bytes())
	l.buf.Reset()
}

func (l *Liner) Finish() {
	fmt.Fprintln(l.Writer)
}
