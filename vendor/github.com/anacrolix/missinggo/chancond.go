package missinggo

import "sync"

type ChanCond struct {
	mu sync.Mutex
	ch chan struct{}
}

func (me *ChanCond) Wait() <-chan struct{} {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.ch == nil {
		me.ch = make(chan struct{})
	}
	return me.ch
}

func (me *ChanCond) Signal() {
	me.mu.Lock()
	defer me.mu.Unlock()
	select {
	case me.ch <- struct{}{}:
	default:
	}
}

func (me *ChanCond) Broadcast() {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.ch == nil {
		return
	}
	close(me.ch)
	me.ch = nil
}
