package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"net/http"
	"net/url"

	"gopkg.in/logex.v1"
)

const (
	H_ETAG                = "Etag"
	H_ACCEPT_RANGES       = "Accept-Ranges"
	H_CONTENT_LENGTH      = "Content-Length"
	H_CONTENT_DISPOSITION = "Content-Disposition"
	H_RANGE               = "Range"
	H_SOURCE              = "X-Source"
)

type Meta struct {
	Pwd      string
	Name     string
	Source   string
	EndPoint string
	Etag     string
	FileSize int64
	BlkBit   uint
	BlkSize  int
	Blocks   Blocks

	header  http.Header
	written int64

	file *os.File
	enc  *json.Encoder
	sync.Mutex
}

func (m *Meta) CopyFrom(mm *Meta) {
	m.file = mm.file
	m.enc = mm.enc
	m.header = mm.header
}

type Blocks []*Block

func (b Blocks) String() string {
	return fmt.Sprintf("<block:%v>", len(b))
}

func NewMetaFormFile(target string) (*Meta, error) {
	f, err := os.Open(target)
	if err != nil {
		return nil, logex.Trace(err)
	}
	defer f.Close()
	m := new(Meta)
	if err := m.Decode(f); err != nil {
		return nil, logex.Trace(err)
	}
	return m, nil
}

func NewMeta(pwd, endPoint string, bit uint, cln bool) (*Meta, error) {
	u, _ := url.Parse(endPoint)

	m := &Meta{
		Pwd:      pwd,
		Name:     path.Base(u.Path),
		Source:   endPoint,
		EndPoint: endPoint,
		BlkBit:   bit,
		BlkSize:  1 << bit,
	}
	if err := m.openFile(cln); err != nil {
		return nil, logex.Trace(err)
	}
	return m, nil
}

func (m *Meta) IsAccpetRange() bool {
	for _, k := range m.header[H_ACCEPT_RANGES] {
		if k == "bytes" {
			return true
		}
	}
	return false
}

func (m *Meta) openFile(cln bool) error {
	if m.file != nil {
		m.file.Close()
	}
	flag := os.O_WRONLY | os.O_APPEND | os.O_CREATE
	if cln {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(m.getDiskPath(), flag, 0666)
	if err != nil {
		return logex.Trace(err)
	}
	m.file = f
	m.enc = json.NewEncoder(m.file)
	return nil
}

func (m *Meta) Close() error {
	return m.file.Close()
}

func (m *Meta) targetPath() string {
	return filepath.Join(m.Pwd, m.Name)
}

func (m *Meta) parseDisposition(dispositions []string) {
	prefix := `filename=`
	for _, d := range dispositions {
		start := strings.Index(d, prefix)
		if start < 0 {
			continue
		}
		start += len(prefix)
		name := d[start:]
		end := strings.Index(name, `"`)
		if end > 0 {
			name = name[:end]
		}
		if len(name) > 0 && name[0] != '"' {
			name = `"` + name + `"`
		}
		fileName, err := strconv.Unquote(name)
		if err != nil {
			continue
		}
		if urlDecode, err := url.QueryUnescape(fileName); err == nil {
			fileName = urlDecode
		}
		m.Name = fileName
		return
	}
}

func (m *Meta) headReq(proxy []string) (*http.Response, error) {
	var ret *http.Response
	var mutex sync.Mutex
	var errInfo []string
	finish := make(chan struct{})
	for i := -1; i < len(proxy); i++ {
		i := i
		go func() {
			var resp *http.Response
			var err error
			if i == -1 {
				resp, err = http.Head(m.Source)
			} else {
				resp, err = http.Head(proxyUrl(proxy[i], m.Source, -1, -1))
				if resp != nil {
					resp.Request.URL, _ = url.Parse(resp.Header.Get(H_SOURCE))
				}
			}
			if err != nil {
				mutex.Lock()
				errInfo = append(errInfo, err.Error())
				if len(errInfo) == len(proxy)+1 {
					finish <- struct{}{}
				}
				mutex.Unlock()
				return
			}
			mutex.Lock()
			if ret == nil {
				ret = resp
				finish <- struct{}{}
			}
			mutex.Unlock()
			resp.Body.Close()
		}()
	}

	<-finish
	if ret != nil {
		return ret, nil
	}
	return nil, errors.New(strings.Join(errInfo, ";"))
}

func (m *Meta) retrieveFromHead(proxy []string) error {
	resp, err := m.headReq(proxy)
	if err != nil {
		return logex.Trace(err)
	}
	m.header = resp.Header
	m.Source = resp.Request.URL.String()

	size, err := strconv.ParseInt(m.header.Get(H_CONTENT_LENGTH), 10, 64)
	if err != nil {
		logex.Error(err)
		return logex.Trace(err)
	}
	if size > 0 {
		m.setFileSize(size)
	}
	m.parseDisposition(m.header[H_CONTENT_DISPOSITION])
	m.Etag = m.header.Get(H_ETAG)
	return nil
}

func (m *Meta) retrieveFromDisk(proxy []string) (err error) {
	if m.header == nil {
		if err = m.retrieveFromHead(proxy); err != nil {
			return logex.Trace(err)
		}
	}

	if !m.IsAccpetRange() {
		return nil
	}

	f, err := os.Open(m.getDiskPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return logex.Trace(err)
	}
	defer f.Close()

	diskMeta, err := NewMeta(m.Pwd, m.EndPoint, m.BlkBit, false)
	if err != nil {
		panic(err)
	}
	if err := diskMeta.Decode(f); err != nil {
		if logex.Equal(err, io.EOF) {
			err = nil
		}
		return logex.Trace(err)
	}

	if diskMeta.BlkBit != m.BlkBit {
		logex.Info("blksize change to", diskMeta.BlkBit)
	}

	if diskMeta.Etag != m.Etag {
		logex.Info("etag not matched, redownload", diskMeta.Etag, m.Etag)
		return nil
	}

	diskMeta.CopyFrom(m)
	diskMeta.BlkSize = 1 << diskMeta.BlkBit
	*m = *diskMeta
	return nil
}

func (m *Meta) Remove() error {
	return logex.Trace(os.Remove(m.getDiskPath()))
}

func (m *Meta) IsFinish() bool {
	return m.FileSize == atomic.LoadInt64(&m.written)
}

func (m *Meta) Sync() error {
	m.Lock()
	defer m.Unlock()

	tmp := m.getDiskPath() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return logex.Trace(err)
	}
	defer f.Close()

	if err := m.Encode(f); err != nil {
		return logex.Trace(err)
	}
	if err := os.Rename(tmp, m.getDiskPath()); err != nil {
		return logex.Trace(err)
	}
	return logex.Trace(m.openFile(false))
}

func (m *Meta) getDiskPath() string {
	return filepath.Join(m.Pwd, fmt.Sprintf("%v.godl", m.Name))
}

func (m *Meta) setFileSize(size int64) *Meta {
	m.FileSize = size
	m.Blocks = make([]*Block, m.BlkCnt())
	return m
}

const (
	STATE_INIT = iota
	STATE_PROCESS
	STATE_FIN
)

type Block struct {
	State   int
	Written int
}

func (b *Block) markFinish(written, max int) (change int64) {
	change = int64(written - b.Written)
	if change < 0 {
		logex.Errorf(
			"wantW: %v, originW: %v, change: %v, state: %v\n",
			written, b.Written, change, b.State,
		)
	}
	b.Written = written
	if written == max {
		b.State = STATE_FIN
	}
	return change
}

func NewBlock() *Block {
	return new(Block)
}

type BlkOff struct {
	Offset  int `json:"o"`
	Written int `json:"w"`
}

func (m *Meta) BlkCnt() int {
	cnt := m.FileSize >> m.BlkBit
	if m.FileSize&int64(m.BlkSize-1) == 0 {
		return int(cnt)
	}
	return int(cnt) + 1
}

func (m *Meta) headers() []interface{} {
	return []interface{}{
		&m.Pwd, &m.Name, &m.Etag, &m.Source,
		&m.FileSize, &m.BlkBit, &m.EndPoint,
	}
}

// store meta into file using gob encoding
func (m *Meta) Decode(r io.Reader) error {
	dec := json.NewDecoder(bufio.NewReader(r))
	for _, d := range m.headers() {
		if err := dec.Decode(d); err != nil {
			return logex.Trace(err)
		}
	}
	cnt := m.BlkCnt()
	m.Blocks = make([]*Block, cnt)
	blkoff := new(BlkOff)
	for {
		err := dec.Decode(&blkoff)
		if err != nil {
			if logex.Equal(err, io.EOF) {
				break
			}
			return logex.Trace(err)
		}
		blk := &Block{
			Written: blkoff.Written,
		}
		if blk.Written == m.BlkSize {
			blk.State = STATE_FIN
		}
		oldblk := m.Blocks[blkoff.Offset]
		m.Blocks[blkoff.Offset] = blk
		if oldblk != nil {
			atomic.AddInt64(&m.written, int64(blk.Written-oldblk.Written))
		} else {
			atomic.AddInt64(&m.written, int64(blk.Written))
		}
	}
	if blk := m.Blocks[len(m.Blocks)-1]; blk != nil {
		if blk.State != STATE_FIN {
			if int(m.FileSize&int64(m.BlkSize-1)) == blk.Written {
				blk.State = STATE_FIN
			}
		}
	}
	return nil
}

func (m *Meta) writeBlock(enc *json.Encoder, blkoff *BlkOff, i int) error {
	var buf *bytes.Buffer
	if enc == nil {
		buf = bytes.NewBuffer(nil)
		enc = json.NewEncoder(buf)
	}
	if blkoff == nil {
		blkoff = new(BlkOff)
	}
	blkoff.Offset = i
	blkoff.Written = m.Blocks[i].Written
	if err := enc.Encode(blkoff); err != nil {
		return logex.Trace(err)
	}
	if buf != nil && len(buf.Bytes()) > 0 {
		if _, err := m.file.Write(buf.Bytes()); err != nil {
			return logex.Trace(err)
		}
	}

	return nil
}

func (m *Meta) Encode(w io.Writer) error {
	buf := bufio.NewWriter(w)
	enc := json.NewEncoder(buf)
	for _, d := range m.headers() {
		if err := enc.Encode(d); err != nil {
			return logex.Trace(err)
		}
	}
	var blkoff BlkOff
	for i := 0; i < len(m.Blocks); i++ {
		if m.Blocks[i] == nil {
			continue
		}
		if err := m.writeBlock(enc, &blkoff, i); err != nil {
			return logex.Trace(err)
		}
	}

	buf.Flush()
	return nil
}

func (m *Meta) MarkFinishStream(written int64) {
	atomic.AddInt64(&m.written, written)
}

func (m *Meta) MarkInit(idx int) {
	m.Blocks[idx].Written = 0
	m.Blocks[idx].State = STATE_INIT
}

func (m *Meta) MarkFinishByN(n int64, lastWritten int, flush bool) error {
	idx := int(n >> m.BlkBit)
	written := int(n - int64(idx<<m.BlkBit))
	if lastWritten > 0 && written == 0 {
		idx--
		written = m.BlkSize
	}

	if written == 0 {
		logex.Error(idx, n, written, lastWritten, m.BlkSize)
	}

	return m.MarkFinish(idx, written, flush)
}

func (m *Meta) MarkFinish(idx, written int, flush bool) error {
	leave := m.FileSize - int64(idx<<m.BlkBit)
	max := m.BlkSize
	if leave < int64(m.BlkSize) {
		max = int(leave)
	}
	change := m.Blocks[idx].markFinish(written, max)
	if change < 0 {
		panic(fmt.Sprintf(
			"idx: %v",
			idx,
		))
	}
	atomic.AddInt64(&m.written, change)
	if flush {
		return logex.Trace(m.writeBlock(nil, nil, idx))
	}
	return nil
}
