package pubsub

import (
	"sync"
)

type PubSub struct {
	mu     sync.Mutex
	next   chan item
	closed bool
}

type item struct {
	value interface{}
	next  chan item
}

type Subscription struct {
	next   chan item
	Values chan interface{}
	mu     sync.Mutex
	closed chan struct{}
}

func NewPubSub() (ret *PubSub) {
	return new(PubSub)
}

func (me *PubSub) init() {
	me.next = make(chan item, 1)
}

func (me *PubSub) lazyInit() {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.closed {
		return
	}
	if me.next == nil {
		me.init()
	}
}

func (me *PubSub) Publish(v interface{}) {
	me.lazyInit()
	next := make(chan item, 1)
	i := item{v, next}
	me.mu.Lock()
	if !me.closed {
		me.next <- i
		me.next = next
	}
	me.mu.Unlock()
}

func (me *Subscription) Close() {
	me.mu.Lock()
	defer me.mu.Unlock()
	select {
	case <-me.closed:
	default:
		close(me.closed)
	}
}

func (me *Subscription) runner() {
	defer close(me.Values)
	for {
		select {
		case i, ok := <-me.next:
			if !ok {
				me.Close()
				return
			}
			me.next <- i
			me.next = i.next
			select {
			case me.Values <- i.value:
			case <-me.closed:
				return
			}
		case <-me.closed:
			return
		}
	}
}

func (me *PubSub) Subscribe() (ret *Subscription) {
	me.lazyInit()
	ret = &Subscription{
		closed: make(chan struct{}),
		Values: make(chan interface{}),
	}
	me.mu.Lock()
	ret.next = me.next
	me.mu.Unlock()
	go ret.runner()
	return
}

func (me *PubSub) Close() {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.closed {
		return
	}
	if me.next != nil {
		close(me.next)
	}
	me.closed = true
}
