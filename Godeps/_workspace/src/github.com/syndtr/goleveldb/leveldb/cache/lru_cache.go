// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cache

import (
	"sync"
	"sync/atomic"
)

// lruCache represent a LRU cache state.
type lruCache struct {
	sync.Mutex

	recent   lruNode
	table    map[uint64]*lruNs
	capacity int
	size     int
}

// NewLRUCache creates a new initialized LRU cache with the given capacity.
func NewLRUCache(capacity int) Cache {
	c := &lruCache{
		table:    make(map[uint64]*lruNs),
		capacity: capacity,
	}
	c.recent.rNext = &c.recent
	c.recent.rPrev = &c.recent
	return c
}

// SetCapacity set cache capacity.
func (c *lruCache) SetCapacity(capacity int) {
	c.Lock()
	c.capacity = capacity
	c.evict()
	c.Unlock()
}

// GetNamespace return namespace object for given id.
func (c *lruCache) GetNamespace(id uint64) Namespace {
	c.Lock()
	defer c.Unlock()

	if p, ok := c.table[id]; ok {
		return p
	}

	p := &lruNs{
		lru:   c,
		id:    id,
		table: make(map[uint64]*lruNode),
	}
	c.table[id] = p
	return p
}

// Purge purge entire cache.
func (c *lruCache) Purge(fin PurgeFin) {
	c.Lock()
	for _, ns := range c.table {
		ns.purgeNB(fin)
	}
	c.Unlock()
}

func (c *lruCache) Zap(closed bool) {
	c.Lock()
	for _, ns := range c.table {
		ns.zapNB(closed)
	}
	c.table = make(map[uint64]*lruNs)
	c.Unlock()
}

func (c *lruCache) evict() {
	top := &c.recent
	for n := c.recent.rPrev; c.size > c.capacity && n != top; {
		n.state = nodeEvicted
		n.rRemove()
		n.evictNB()
		c.size -= n.charge
		n = c.recent.rPrev
	}
}

type lruNs struct {
	lru   *lruCache
	id    uint64
	table map[uint64]*lruNode
	state nsState
}

func (ns *lruNs) Get(key uint64, setf SetFunc) (o Object, ok bool) {
	lru := ns.lru
	lru.Lock()

	switch ns.state {
	case nsZapped:
		lru.Unlock()
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
		lru.Unlock()
		return
	}

	n, ok := ns.table[key]
	if ok {
		switch n.state {
		case nodeEvicted:
			// Insert to recent list.
			n.state = nodeEffective
			n.ref++
			lru.size += n.charge
			lru.evict()
			fallthrough
		case nodeEffective:
			// Bump to front
			n.rRemove()
			n.rInsert(&lru.recent)
		}
		n.ref++
	} else {
		if setf == nil {
			lru.Unlock()
			return
		}

		var value interface{}
		var charge int
		var fin func()
		ok, value, charge, fin = setf()
		if !ok {
			lru.Unlock()
			return
		}

		n = &lruNode{
			ns:     ns,
			key:    key,
			value:  value,
			charge: charge,
			setfin: fin,
			ref:    2,
		}
		ns.table[key] = n
		n.rInsert(&lru.recent)

		lru.size += charge
		lru.evict()
	}

	lru.Unlock()
	o = &lruObject{node: n}
	return
}

func (ns *lruNs) Delete(key uint64, fin DelFin) bool {
	lru := ns.lru
	lru.Lock()

	if ns.state != nsEffective {
		lru.Unlock()
		if fin != nil {
			fin(false)
		}
		return false
	}

	n, ok := ns.table[key]
	if !ok {
		lru.Unlock()
		if fin != nil {
			fin(false)
		}
		return false
	}

	n.delfin = fin
	switch n.state {
	case nodeRemoved:
		lru.Unlock()
		return false
	case nodeEffective:
		lru.size -= n.charge
		n.rRemove()
		n.evictNB()
	}
	n.state = nodeRemoved

	lru.Unlock()
	return true
}

func (ns *lruNs) purgeNB(fin PurgeFin) {
	lru := ns.lru
	if ns.state != nsEffective {
		return
	}

	for _, n := range ns.table {
		n.purgefin = fin
		if n.state == nodeEffective {
			lru.size -= n.charge
			n.rRemove()
			n.evictNB()
		}
		n.state = nodeRemoved
	}
}

func (ns *lruNs) Purge(fin PurgeFin) {
	ns.lru.Lock()
	ns.purgeNB(fin)
	ns.lru.Unlock()
}

func (ns *lruNs) zapNB(closed bool) {
	lru := ns.lru
	if ns.state != nsEffective {
		return
	}

	if closed {
		ns.state = nsClosed
	} else {
		ns.state = nsZapped
	}
	for _, n := range ns.table {
		if n.state == nodeEffective {
			lru.size -= n.charge
			n.rRemove()
		}
		n.state = nodeRemoved
		n.execFin()
	}
	ns.table = nil
}

func (ns *lruNs) Zap(closed bool) {
	ns.lru.Lock()
	ns.zapNB(closed)
	delete(ns.lru.table, ns.id)
	ns.lru.Unlock()
}

type lruNode struct {
	ns *lruNs

	rNext, rPrev *lruNode

	key      uint64
	value    interface{}
	charge   int
	ref      int
	state    nodeState
	setfin   SetFin
	delfin   DelFin
	purgefin PurgeFin
}

func (n *lruNode) rInsert(at *lruNode) {
	x := at.rNext
	at.rNext = n
	n.rPrev = at
	n.rNext = x
	x.rPrev = n
}

func (n *lruNode) rRemove() bool {
	// only remove if not already removed
	if n.rPrev == nil {
		return false
	}

	n.rPrev.rNext = n.rNext
	n.rNext.rPrev = n.rPrev
	n.rPrev = nil
	n.rNext = nil

	return true
}

func (n *lruNode) execFin() {
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

func (n *lruNode) evictNB() {
	n.ref--
	if n.ref == 0 {
		if n.ns.state == nsEffective {
			// remove elem
			delete(n.ns.table, n.key)
			// execute finalizer
			n.execFin()
		}
	} else if n.ref < 0 {
		panic("leveldb/cache: lruCache: negative node reference")
	}
}

func (n *lruNode) evict() {
	n.ns.lru.Lock()
	n.evictNB()
	n.ns.lru.Unlock()
}

type lruObject struct {
	node *lruNode
	once uint32
}

func (o *lruObject) Value() interface{} {
	if atomic.LoadUint32(&o.once) == 0 {
		return o.node.value
	}
	return nil
}

func (o *lruObject) Release() {
	if !atomic.CompareAndSwapUint32(&o.once, 0, 1) {
		return
	}

	o.node.evict()
	o.node = nil
}
