// Package prioritybitmap implements a set of integers ordered by attached
// priorities.
package prioritybitmap

import (
	"sync"

	"github.com/anacrolix/missinggo/orderedmap"
)

var (
	bitSets = sync.Pool{
		New: func() interface{} {
			return make(map[int]struct{}, 1)
		},
	}
)

type PriorityBitmap struct {
	// Protects against unsychronized modifications to bitsets and
	mu sync.Mutex
	om orderedmap.OrderedMap
	// From bit index to priority
	priorities map[int]int
}

func (me *PriorityBitmap) Clear() {
	me.om = nil
	me.priorities = nil
}

func (me *PriorityBitmap) deleteBit(bit int) {
	p, ok := me.priorities[bit]
	if !ok {
		return
	}
	switch v := me.om.Get(p).(type) {
	case int:
	case map[int]struct{}:
		delete(v, bit)
		if len(v) != 0 {
			return
		}
		bitSets.Put(v)
	}
	me.om.Unset(p)
	if me.om.Len() == 0 {
		me.om = nil
	}
}

func bitLess(l, r interface{}) bool {
	return l.(int) < r.(int)
}

func (me *PriorityBitmap) lazyInit() {
	me.om = orderedmap.New(func(l, r interface{}) bool {
		return l.(int) < r.(int)
	})
	me.priorities = make(map[int]int)
}

func (me *PriorityBitmap) Set(bit int, priority int) {
	me.deleteBit(bit)
	if me.priorities == nil {
		me.priorities = make(map[int]int)
	}
	me.priorities[bit] = priority
	if me.om == nil {
		me.om = orderedmap.New(bitLess)
	}
	_v, ok := me.om.GetOk(priority)
	if !ok {
		me.om.Set(priority, bit)
		return
	}
	switch v := _v.(type) {
	case int:
		newV := bitSets.Get().(map[int]struct{})
		newV[v] = struct{}{}
		newV[bit] = struct{}{}
		me.om.Set(priority, newV)
	case map[int]struct{}:
		v[bit] = struct{}{}
	}
}

func (me *PriorityBitmap) Remove(bit int) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.deleteBit(bit)
	delete(me.priorities, bit)
	if len(me.priorities) == 0 {
		me.priorities = nil
	}
	if me.om != nil && me.om.Len() == 0 {
		me.om = nil
	}
}

func (me *PriorityBitmap) Iter(f func(value interface{}) bool) {
	me.IterTyped(func(i int) bool {
		return f(i)
	})
}

func (me *PriorityBitmap) IterTyped(_f func(i int) bool) {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me == nil || me.om == nil {
		return
	}
	f := func(i int) bool {
		me.mu.Unlock()
		defer me.mu.Lock()
		return _f(i)
	}
	me.om.Iter(func(value interface{}) bool {
		switch v := value.(type) {
		case int:
			return f(v)
		case map[int]struct{}:
			for i := range v {
				if !f(i) {
					return false
				}
			}
		}
		return true
	})
}

func (me *PriorityBitmap) IsEmpty() bool {
	if me.om == nil {
		return true
	}
	return me.om.Len() == 0
}
