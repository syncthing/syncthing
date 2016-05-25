package itertools

import (
	"sync"

	"github.com/anacrolix/missinggo"
)

type Iterable interface {
	Iter(func(value interface{}) (more bool))
}

type iterator struct {
	it      Iterable
	ch      chan interface{}
	value   interface{}
	ok      bool
	mu      sync.Mutex
	stopped missinggo.Event
}

func NewIterator(it Iterable) (ret *iterator) {
	ret = &iterator{
		it: it,
		ch: make(chan interface{}),
	}
	go func() {
		// Have to do this in a goroutine, because the interface is synchronous.
		it.Iter(func(value interface{}) bool {
			select {
			case ret.ch <- value:
				return true
			case <-ret.stopped.LockedChan(&ret.mu):
				return false
			}
		})
		close(ret.ch)
		ret.mu.Lock()
		ret.stopped.Set()
		ret.mu.Unlock()
	}()
	return
}

func (me *iterator) Value() interface{} {
	if !me.ok {
		panic("no value")
	}
	return me.value
}

func (me *iterator) Next() bool {
	me.value, me.ok = <-me.ch
	return me.ok
}

func (me *iterator) Stop() {
	me.mu.Lock()
	me.stopped.Set()
	me.mu.Unlock()
}

func IterableAsSlice(it Iterable) (ret []interface{}) {
	it.Iter(func(value interface{}) bool {
		ret = append(ret, value)
		return true
	})
	return
}
