// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cache

import (
	"sync"
	"sync/atomic"
)

type emptyCache struct {
	sync.Mutex
	table map[uint64]*emptyNS
}

// NewEmptyCache creates a new initialized empty cache.
func NewEmptyCache() Cache {
	return &emptyCache{
		table: make(map[uint64]*emptyNS),
	}
}

func (c *emptyCache) GetNamespace(id uint64) Namespace {
	c.Lock()
	defer c.Unlock()

	if ns, ok := c.table[id]; ok {
		return ns
	}

	ns := &emptyNS{
		cache: c,
		id:    id,
		table: make(map[uint64]*emptyNode),
	}
	c.table[id] = ns
	return ns
}

func (c *emptyCache) Purge(fin PurgeFin) {
	c.Lock()
	for _, ns := range c.table {
		ns.purgeNB(fin)
	}
	c.Unlock()
}

func (c *emptyCache) Zap(closed bool) {
	c.Lock()
	for _, ns := range c.table {
		ns.zapNB(closed)
	}
	c.table = make(map[uint64]*emptyNS)
	c.Unlock()
}

func (*emptyCache) SetCapacity(capacity int) {}

type emptyNS struct {
	cache *emptyCache
	id    uint64
	table map[uint64]*emptyNode
	state nsState
}

func (ns *emptyNS) Get(key uint64, setf SetFunc) (o Object, ok bool) {
	ns.cache.Lock()

	switch ns.state {
	case nsZapped:
		ns.cache.Unlock()
		if setf == nil {
			return
		}

		var value interface{}
		var fin func()
		ok, value, _, fin = setf()
		if ok {
			o = &fakeObject{
				value: value,
				fin:   fin,
			}
		}
		return
	case nsClosed:
		ns.cache.Unlock()
		return
	}

	n, ok := ns.table[key]
	if ok {
		n.ref++
	} else {
		if setf == nil {
			ns.cache.Unlock()
			return
		}

		var value interface{}
		var fin func()
		ok, value, _, fin = setf()
		if !ok {
			ns.cache.Unlock()
			return
		}

		n = &emptyNode{
			ns:     ns,
			key:    key,
			value:  value,
			setfin: fin,
			ref:    1,
		}
		ns.table[key] = n
	}

	ns.cache.Unlock()
	o = &emptyObject{node: n}
	return
}

func (ns *emptyNS) Delete(key uint64, fin DelFin) bool {
	ns.cache.Lock()

	if ns.state != nsEffective {
		ns.cache.Unlock()
		if fin != nil {
			fin(false)
		}
		return false
	}

	n, ok := ns.table[key]
	if !ok {
		ns.cache.Unlock()
		if fin != nil {
			fin(false)
		}
		return false
	}
	n.delfin = fin
	ns.cache.Unlock()
	return true
}

func (ns *emptyNS) purgeNB(fin PurgeFin) {
	if ns.state != nsEffective {
		return
	}
	for _, n := range ns.table {
		n.purgefin = fin
	}
}

func (ns *emptyNS) Purge(fin PurgeFin) {
	ns.cache.Lock()
	ns.purgeNB(fin)
	ns.cache.Unlock()
}

func (ns *emptyNS) zapNB(closed bool) {
	if ns.state != nsEffective {
		return
	}
	for _, n := range ns.table {
		n.execFin()
	}
	if closed {
		ns.state = nsClosed
	} else {
		ns.state = nsZapped
	}
	ns.table = nil
}

func (ns *emptyNS) Zap(closed bool) {
	ns.cache.Lock()
	ns.zapNB(closed)
	delete(ns.cache.table, ns.id)
	ns.cache.Unlock()
}

type emptyNode struct {
	ns       *emptyNS
	key      uint64
	value    interface{}
	ref      int
	setfin   SetFin
	delfin   DelFin
	purgefin PurgeFin
}

func (n *emptyNode) execFin() {
	if n.setfin != nil {
		n.setfin()
		n.setfin = nil
	}
	if n.purgefin != nil {
		n.purgefin(n.ns.id, n.key, n.delfin)
		n.delfin = nil
		n.purgefin = nil
	} else if n.delfin != nil {
		n.delfin(true)
		n.delfin = nil
	}
}

func (n *emptyNode) evict() {
	n.ns.cache.Lock()
	n.ref--
	if n.ref == 0 {
		if n.ns.state == nsEffective {
			// Remove elem.
			delete(n.ns.table, n.key)
			// Execute finalizer.
			n.execFin()
		}
	} else if n.ref < 0 {
		panic("leveldb/cache: emptyNode: negative node reference")
	}
	n.ns.cache.Unlock()
}

type emptyObject struct {
	node *emptyNode
	once uint32
}

func (o *emptyObject) Value() interface{} {
	if atomic.LoadUint32(&o.once) == 0 {
		return o.node.value
	}
	return nil
}

func (o *emptyObject) Release() {
	if !atomic.CompareAndSwapUint32(&o.once, 0, 1) {
		return
	}
	o.node.evict()
	o.node = nil
}
