package missinggo

import "sync"

// Events are boolean flags that provide a channel that's closed when true.
type Event struct {
	ch     chan struct{}
	closed bool
}

func (me *Event) LockedChan(lock sync.Locker) <-chan struct{} {
	lock.Lock()
	ch := me.C()
	lock.Unlock()
	return ch
}

func (me *Event) C() <-chan struct{} {
	if me.ch == nil {
		me.ch = make(chan struct{})
	}
	return me.ch
}

func (me *Event) Clear() {
	if me.closed {
		me.ch = nil
		me.closed = false
	}
}

func (me *Event) Set() (first bool) {
	if me.closed {
		return false
	}
	if me.ch == nil {
		me.ch = make(chan struct{})
	}
	close(me.ch)
	me.closed = true
	return true
}

func (me *Event) IsSet() bool {
	return me.closed
}

func (me *Event) Wait() {
	<-me.C()
}

func (me *Event) SetBool(b bool) {
	if b {
		me.Set()
	} else {
		me.Clear()
	}
}
