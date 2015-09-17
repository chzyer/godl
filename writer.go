package main

import "gopkg.in/logex.v1"

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
