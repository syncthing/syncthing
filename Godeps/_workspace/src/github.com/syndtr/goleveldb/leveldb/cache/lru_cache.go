// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cache

import (
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/util"
)

// lruCache represent a LRU cache state.
type lruCache struct {
	mu         sync.Mutex
	recent     lruNode
	table      map[uint64]*lruNs
	capacity   int
	used, size int
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

func (c *lruCache) Capacity() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capacity
}

func (c *lruCache) Used() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}

func (c *lruCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.size
}

// SetCapacity set cache capacity.
func (c *lruCache) SetCapacity(capacity int) {
	c.mu.Lock()
	c.capacity = capacity
	c.evict()
	c.mu.Unlock()
}

// GetNamespace return namespace object for given id.
func (c *lruCache) GetNamespace(id uint64) Namespace {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ns, ok := c.table[id]; ok {
		return ns
	}

	ns := &lruNs{
		lru:   c,
		id:    id,
		table: make(map[uint64]*lruNode),
	}
	c.table[id] = ns
	return ns
}

// Purge purge entire cache.
func (c *lruCache) Purge(fin PurgeFin) {
	c.mu.Lock()
	for _, ns := range c.table {
		ns.purgeNB(fin)
	}
	c.mu.Unlock()
}

func (c *lruCache) Zap() {
	c.mu.Lock()
	for _, ns := range c.table {
		ns.zapNB()
	}
	c.table = make(map[uint64]*lruNs)
	c.mu.Unlock()
}

func (c *lruCache) evict() {
	top := &c.recent
	for n := c.recent.rPrev; c.used > c.capacity && n != top; {
		n.state = nodeEvicted
		n.rRemove()
		n.derefNB()
		c.used -= n.charge
		n = c.recent.rPrev
	}
}

type lruNs struct {
	lru   *lruCache
	id    uint64
	table map[uint64]*lruNode
	state nsState
}

func (ns *lruNs) Get(key uint64, setf SetFunc) Handle {
	ns.lru.mu.Lock()

	if ns.state != nsEffective {
		ns.lru.mu.Unlock()
		return nil
	}

	node, ok := ns.table[key]
	if ok {
		switch node.state {
		case nodeEvicted:
			// Insert to recent list.
			node.state = nodeEffective
			node.ref++
			ns.lru.used += node.charge
			ns.lru.evict()
			fallthrough
		case nodeEffective:
			// Bump to front.
			node.rRemove()
			node.rInsert(&ns.lru.recent)
		}
		node.ref++
	} else {
		if setf == nil {
			ns.lru.mu.Unlock()
			return nil
		}

		charge, value := setf()
		if value == nil {
			ns.lru.mu.Unlock()
			return nil
		}

		node = &lruNode{
			ns:     ns,
			key:    key,
			value:  value,
			charge: charge,
			ref:    1,
		}
		ns.table[key] = node

		if charge > 0 {
			node.ref++
			node.rInsert(&ns.lru.recent)
			ns.lru.used += charge
			ns.lru.size += charge
			ns.lru.evict()
		}
	}

	ns.lru.mu.Unlock()
	return &lruHandle{node: node}
}

func (ns *lruNs) Delete(key uint64, fin DelFin) bool {
	ns.lru.mu.Lock()

	if ns.state != nsEffective {
		if fin != nil {
			fin(false, false)
		}
		ns.lru.mu.Unlock()
		return false
	}

	node, exist := ns.table[key]
	if !exist {
		if fin != nil {
			fin(false, false)
		}
		ns.lru.mu.Unlock()
		return false
	}

	switch node.state {
	case nodeDeleted:
		if fin != nil {
			fin(true, true)
		}
		ns.lru.mu.Unlock()
		return false
	case nodeEffective:
		ns.lru.used -= node.charge
		node.state = nodeDeleted
		node.delfin = fin
		node.rRemove()
		node.derefNB()
	default:
		node.state = nodeDeleted
		node.delfin = fin
	}

	ns.lru.mu.Unlock()
	return true
}

func (ns *lruNs) purgeNB(fin PurgeFin) {
	if ns.state != nsEffective {
		return
	}

	for _, node := range ns.table {
		switch node.state {
		case nodeDeleted:
		case nodeEffective:
			ns.lru.used -= node.charge
			node.state = nodeDeleted
			node.purgefin = fin
			node.rRemove()
			node.derefNB()
		default:
			node.state = nodeDeleted
			node.purgefin = fin
		}
	}
}

func (ns *lruNs) Purge(fin PurgeFin) {
	ns.lru.mu.Lock()
	ns.purgeNB(fin)
	ns.lru.mu.Unlock()
}

func (ns *lruNs) zapNB() {
	if ns.state != nsEffective {
		return
	}

	ns.state = nsZapped

	for _, node := range ns.table {
		if node.state == nodeEffective {
			ns.lru.used -= node.charge
			node.rRemove()
		}
		ns.lru.size -= node.charge
		node.state = nodeDeleted
		node.fin()
	}
	ns.table = nil
}

func (ns *lruNs) Zap() {
	ns.lru.mu.Lock()
	ns.zapNB()
	delete(ns.lru.table, ns.id)
	ns.lru.mu.Unlock()
}

type lruNode struct {
	ns *lruNs

	rNext, rPrev *lruNode

	key      uint64
	value    interface{}
	charge   int
	ref      int
	state    nodeState
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
	if n.rPrev == nil {
		return false
	}

	n.rPrev.rNext = n.rNext
	n.rNext.rPrev = n.rPrev
	n.rPrev = nil
	n.rNext = nil

	return true
}

func (n *lruNode) fin() {
	if r, ok := n.value.(util.Releaser); ok {
		r.Release()
	}
	if n.purgefin != nil {
		n.purgefin(n.ns.id, n.key)
		n.delfin = nil
		n.purgefin = nil
	} else if n.delfin != nil {
		n.delfin(true, false)
		n.delfin = nil
	}
}

func (n *lruNode) derefNB() {
	n.ref--
	if n.ref == 0 {
		if n.ns.state == nsEffective {
			// Remove elemement.
			delete(n.ns.table, n.key)
			n.ns.lru.size -= n.charge
			n.fin()
		}
	} else if n.ref < 0 {
		panic("leveldb/cache: lruCache: negative node reference")
	}
}

func (n *lruNode) deref() {
	n.ns.lru.mu.Lock()
	n.derefNB()
	n.ns.lru.mu.Unlock()
}

type lruHandle struct {
	node *lruNode
	once uint32
}

func (h *lruHandle) Value() interface{} {
	if atomic.LoadUint32(&h.once) == 0 {
		return h.node.value
	}
	return nil
}

func (h *lruHandle) Release() {
	if !atomic.CompareAndSwapUint32(&h.once, 0, 1) {
		return
	}
	h.node.deref()
	h.node = nil
}
