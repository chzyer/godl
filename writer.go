package main

import (
	"io"
	"sync/atomic"

	"gopkg.in/logex.v1"
)

type onWriteFunc func(int64, int) error

type FileWriter struct {
	Offset  int64
	first   bool
	op      *writeOp
	onWrite onWriteFunc
	ch      chan *writeOp
}

func NewFileWriter(task *DnTask, off int64, op *writeOp, ch chan *writeOp, onWrite onWriteFunc) *FileWriter {
	return &FileWriter{
		Offset:  off,
		onWrite: onWrite,
		first:   true,
		op:      op,
		ch:      ch,
	}
}

func (w *FileWriter) Write(buf []byte) (int, error) {
	// (*bufio.Reader).WriteTo cause empty buf write at first
	if w.first {
		w.first = false
		if len(buf) == 0 {
			return 0, nil
		}
	}
	w.op.Buf = buf
	w.op.Offset = w.Offset
	w.ch <- w.op
	reply := <-w.op.Reply
	w.Offset += int64(reply.N)
	if reply.Err != nil {
		return reply.N, logex.Trace(reply.Err)
	}
	err := w.onWrite(w.Offset, reply.N)
	return reply.N, logex.Trace(err)
}

var report int64

type Reader struct {
	r      io.Reader
	closed int64
}

func NewReader(r io.Reader) *Reader {
	rr := &Reader{r: r}
	return rr
}

func (r *Reader) Read(b []byte) (int, error) {
	n, err := r.r.Read(b)
	atomic.AddInt64(&report, int64(n))
	return n, err
}

func (r *Reader) Close() error {
	if rc := r.r.(io.ReadCloser); rc != nil {
		return rc.Close()
	}
	atomic.StoreInt64(&report, 1)
	return nil
}
