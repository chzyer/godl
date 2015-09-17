package main

import (
	"sync"
	"sync/atomic"
)

type RateLimit struct {
	written int64
	max     int64
	waiting int64
	wg      sync.WaitGroup
	sync.Mutex
}

func NewRateLimit(max int64) *RateLimit {
	r := new(RateLimit)
	r.max = max
	return r
}

func (r *RateLimit) Reset() {
	r.Lock()
	if atomic.SwapInt64(&r.waiting, 0) != 0 {
		r.wg.Done()
	}
	atomic.StoreInt64(&r.written, 0)
	r.Unlock()
}

func (r *RateLimit) Wait() {
	r.Lock()
	if atomic.LoadInt64(&r.written) < r.max {
		r.Unlock()
		return
	}
	w := atomic.AddInt64(&r.waiting, 1)
	if w == 1 {
		r.wg.Add(1)
	}
	r.Unlock()

	r.wg.Wait()
}

func (r *RateLimit) Process(w int) {
redo:
	written := atomic.AddInt64(&r.written, int64(w))
	if written > r.max {
		r.Wait()
		goto redo
	}
}
