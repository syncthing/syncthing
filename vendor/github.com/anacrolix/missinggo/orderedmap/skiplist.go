package orderedmap

import "github.com/ryszard/goskiplist/skiplist"

type skiplistOrderedMap struct {
	sl *skiplist.SkipList
}

func NewSkipList(lesser func(l, r interface{}) bool) *skiplistOrderedMap {
	return &skiplistOrderedMap{skiplist.NewCustomMap(lesser)}
}

func (me *skiplistOrderedMap) Set(key interface{}, value interface{}) {
	me.sl.Set(key, value)
}

func (me *skiplistOrderedMap) Get(key interface{}) interface{} {
	if me == nil {
		return nil
	}
	ret, _ := me.sl.Get(key)
	return ret
}

func (me *skiplistOrderedMap) GetOk(key interface{}) (interface{}, bool) {
	if me == nil {
		return nil, false
	}
	return me.sl.Get(key)
}

type Iter struct {
	it skiplist.Iterator
}

func (me *Iter) Next() bool {
	if me == nil {
		return false
	}
	return me.it.Next()
}

func (me *Iter) Value() interface{} {
	return me.it.Value()
}

func (me *skiplistOrderedMap) Iter() *Iter {
	if me == nil {
		return nil
	}
	return &Iter{me.sl.Iterator()}
}

func (me *skiplistOrderedMap) Unset(key interface{}) {
	if me == nil {
		return
	}
	me.sl.Delete(key)
}

func (me *skiplistOrderedMap) Len() int {
	if me.sl == nil {
		return 0
	}
	return me.sl.Len()
}
