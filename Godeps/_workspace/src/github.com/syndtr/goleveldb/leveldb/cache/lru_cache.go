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

// The LLRB implementation were taken from https://github.com/petar/GoLLRB.
// Which contains the following header:
//
// Copyright 2010 Petar Maymounkov. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// lruCache represent a LRU cache state.
type lruCache struct {
	mu                sync.Mutex
	recent            lruNode
	table             map[uint64]*lruNs
	capacity          int
	used, size, alive int
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

func (c *lruCache) NumObjects() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.alive
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

	ns := &lruNs{lru: c, id: id}
	c.table[id] = ns
	return ns
}

func (c *lruCache) ZapNamespace(id uint64) {
	c.mu.Lock()
	if ns, exist := c.table[id]; exist {
		ns.zapNB()
		delete(c.table, id)
	}
	c.mu.Unlock()
}

func (c *lruCache) PurgeNamespace(id uint64, fin PurgeFin) {
	c.mu.Lock()
	if ns, exist := c.table[id]; exist {
		ns.purgeNB(fin)
	}
	c.mu.Unlock()
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
		if n.state != nodeEffective {
			panic("evicting non effective node")
		}
		n.state = nodeEvicted
		n.rRemove()
		n.derefNB()
		c.used -= n.charge
		n = c.recent.rPrev
	}
}

type lruNs struct {
	lru    *lruCache
	id     uint64
	rbRoot *lruNode
	state  nsState
}

func (ns *lruNs) rbGetOrCreateNode(h *lruNode, key uint64) (hn, n *lruNode) {
	if h == nil {
		n = &lruNode{ns: ns, key: key}
		return n, n
	}

	if key < h.key {
		hn, n = ns.rbGetOrCreateNode(h.rbLeft, key)
		if hn != nil {
			h.rbLeft = hn
		} else {
			return nil, n
		}
	} else if key > h.key {
		hn, n = ns.rbGetOrCreateNode(h.rbRight, key)
		if hn != nil {
			h.rbRight = hn
		} else {
			return nil, n
		}
	} else {
		return nil, h
	}

	if rbIsRed(h.rbRight) && !rbIsRed(h.rbLeft) {
		h = rbRotLeft(h)
	}
	if rbIsRed(h.rbLeft) && rbIsRed(h.rbLeft.rbLeft) {
		h = rbRotRight(h)
	}
	if rbIsRed(h.rbLeft) && rbIsRed(h.rbRight) {
		rbFlip(h)
	}
	return h, n
}

func (ns *lruNs) getOrCreateNode(key uint64) *lruNode {
	hn, n := ns.rbGetOrCreateNode(ns.rbRoot, key)
	if hn != nil {
		ns.rbRoot = hn
		ns.rbRoot.rbBlack = true
	}
	return n
}

func (ns *lruNs) rbGetNode(key uint64) *lruNode {
	h := ns.rbRoot
	for h != nil {
		switch {
		case key < h.key:
			h = h.rbLeft
		case key > h.key:
			h = h.rbRight
		default:
			return h
		}
	}
	return nil
}

func (ns *lruNs) getNode(key uint64) *lruNode {
	return ns.rbGetNode(key)
}

func (ns *lruNs) rbDeleteNode(h *lruNode, key uint64) *lruNode {
	if h == nil {
		return nil
	}

	if key < h.key {
		if h.rbLeft == nil { // key not present. Nothing to delete
			return h
		}
		if !rbIsRed(h.rbLeft) && !rbIsRed(h.rbLeft.rbLeft) {
			h = rbMoveLeft(h)
		}
		h.rbLeft = ns.rbDeleteNode(h.rbLeft, key)
	} else {
		if rbIsRed(h.rbLeft) {
			h = rbRotRight(h)
		}
		// If @key equals @h.key and no right children at @h
		if h.key == key && h.rbRight == nil {
			return nil
		}
		if h.rbRight != nil && !rbIsRed(h.rbRight) && !rbIsRed(h.rbRight.rbLeft) {
			h = rbMoveRight(h)
		}
		// If @key equals @h.key, and (from above) 'h.Right != nil'
		if h.key == key {
			var x *lruNode
			h.rbRight, x = rbDeleteMin(h.rbRight)
			if x == nil {
				panic("logic")
			}
			x.rbLeft, h.rbLeft = h.rbLeft, nil
			x.rbRight, h.rbRight = h.rbRight, nil
			x.rbBlack = h.rbBlack
			h = x
		} else { // Else, @key is bigger than @h.key
			h.rbRight = ns.rbDeleteNode(h.rbRight, key)
		}
	}

	return rbFixup(h)
}

func (ns *lruNs) deleteNode(key uint64) {
	ns.rbRoot = ns.rbDeleteNode(ns.rbRoot, key)
	if ns.rbRoot != nil {
		ns.rbRoot.rbBlack = true
	}
}

func (ns *lruNs) rbIterateNodes(h *lruNode, pivot uint64, iter func(n *lruNode) bool) bool {
	if h == nil {
		return true
	}
	if h.key >= pivot {
		if !ns.rbIterateNodes(h.rbLeft, pivot, iter) {
			return false
		}
		if !iter(h) {
			return false
		}
	}
	return ns.rbIterateNodes(h.rbRight, pivot, iter)
}

func (ns *lruNs) iterateNodes(iter func(n *lruNode) bool) {
	ns.rbIterateNodes(ns.rbRoot, 0, iter)
}

func (ns *lruNs) Get(key uint64, setf SetFunc) Handle {
	ns.lru.mu.Lock()
	defer ns.lru.mu.Unlock()

	if ns.state != nsEffective {
		return nil
	}

	var n *lruNode
	if setf == nil {
		n = ns.getNode(key)
		if n == nil {
			return nil
		}
	} else {
		n = ns.getOrCreateNode(key)
	}
	switch n.state {
	case nodeZero:
		charge, value := setf()
		if value == nil {
			ns.deleteNode(key)
			return nil
		}
		if charge < 0 {
			charge = 0
		}

		n.value = value
		n.charge = charge
		n.state = nodeEvicted

		ns.lru.size += charge
		ns.lru.alive++

		fallthrough
	case nodeEvicted:
		if n.charge == 0 {
			break
		}

		// Insert to recent list.
		n.state = nodeEffective
		n.ref++
		ns.lru.used += n.charge
		ns.lru.evict()

		fallthrough
	case nodeEffective:
		// Bump to front.
		n.rRemove()
		n.rInsert(&ns.lru.recent)
	case nodeDeleted:
		// Do nothing.
	default:
		panic("invalid state")
	}
	n.ref++

	return &lruHandle{node: n}
}

func (ns *lruNs) Delete(key uint64, fin DelFin) bool {
	ns.lru.mu.Lock()
	defer ns.lru.mu.Unlock()

	if ns.state != nsEffective {
		if fin != nil {
			fin(false, false)
		}
		return false
	}

	n := ns.getNode(key)
	if n == nil {
		if fin != nil {
			fin(false, false)
		}
		return false

	}

	switch n.state {
	case nodeEffective:
		ns.lru.used -= n.charge
		n.state = nodeDeleted
		n.delfin = fin
		n.rRemove()
		n.derefNB()
	case nodeEvicted:
		n.state = nodeDeleted
		n.delfin = fin
	case nodeDeleted:
		if fin != nil {
			fin(true, true)
		}
		return false
	default:
		panic("invalid state")
	}

	return true
}

func (ns *lruNs) purgeNB(fin PurgeFin) {
	if ns.state == nsEffective {
		var nodes []*lruNode
		ns.iterateNodes(func(n *lruNode) bool {
			nodes = append(nodes, n)
			return true
		})
		for _, n := range nodes {
			switch n.state {
			case nodeEffective:
				ns.lru.used -= n.charge
				n.state = nodeDeleted
				n.purgefin = fin
				n.rRemove()
				n.derefNB()
			case nodeEvicted:
				n.state = nodeDeleted
				n.purgefin = fin
			case nodeDeleted:
			default:
				panic("invalid state")
			}
		}
	}
}

func (ns *lruNs) Purge(fin PurgeFin) {
	ns.lru.mu.Lock()
	ns.purgeNB(fin)
	ns.lru.mu.Unlock()
}

func (ns *lruNs) zapNB() {
	if ns.state == nsEffective {
		ns.state = nsZapped

		ns.iterateNodes(func(n *lruNode) bool {
			if n.state == nodeEffective {
				ns.lru.used -= n.charge
				n.rRemove()
			}
			ns.lru.size -= n.charge
			n.state = nodeDeleted
			n.fin()

			return true
		})
		ns.rbRoot = nil
	}
}

func (ns *lruNs) Zap() {
	ns.lru.mu.Lock()
	ns.zapNB()
	delete(ns.lru.table, ns.id)
	ns.lru.mu.Unlock()
}

type lruNode struct {
	ns *lruNs

	rNext, rPrev    *lruNode
	rbLeft, rbRight *lruNode
	rbBlack         bool

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
		if n.delfin != nil {
			panic("conflicting delete and purge fin")
		}
		n.purgefin(n.ns.id, n.key)
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
			n.ns.deleteNode(n.key)
			n.ns.lru.size -= n.charge
			n.ns.lru.alive--
			n.fin()
		}
		n.value = nil
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

func rbIsRed(h *lruNode) bool {
	if h == nil {
		return false
	}
	return !h.rbBlack
}

func rbRotLeft(h *lruNode) *lruNode {
	x := h.rbRight
	if x.rbBlack {
		panic("rotating a black link")
	}
	h.rbRight = x.rbLeft
	x.rbLeft = h
	x.rbBlack = h.rbBlack
	h.rbBlack = false
	return x
}

func rbRotRight(h *lruNode) *lruNode {
	x := h.rbLeft
	if x.rbBlack {
		panic("rotating a black link")
	}
	h.rbLeft = x.rbRight
	x.rbRight = h
	x.rbBlack = h.rbBlack
	h.rbBlack = false
	return x
}

func rbFlip(h *lruNode) {
	h.rbBlack = !h.rbBlack
	h.rbLeft.rbBlack = !h.rbLeft.rbBlack
	h.rbRight.rbBlack = !h.rbRight.rbBlack
}

func rbMoveLeft(h *lruNode) *lruNode {
	rbFlip(h)
	if rbIsRed(h.rbRight.rbLeft) {
		h.rbRight = rbRotRight(h.rbRight)
		h = rbRotLeft(h)
		rbFlip(h)
	}
	return h
}

func rbMoveRight(h *lruNode) *lruNode {
	rbFlip(h)
	if rbIsRed(h.rbLeft.rbLeft) {
		h = rbRotRight(h)
		rbFlip(h)
	}
	return h
}

func rbFixup(h *lruNode) *lruNode {
	if rbIsRed(h.rbRight) {
		h = rbRotLeft(h)
	}

	if rbIsRed(h.rbLeft) && rbIsRed(h.rbLeft.rbLeft) {
		h = rbRotRight(h)
	}

	if rbIsRed(h.rbLeft) && rbIsRed(h.rbRight) {
		rbFlip(h)
	}

	return h
}

func rbDeleteMin(h *lruNode) (hn, n *lruNode) {
	if h == nil {
		return nil, nil
	}
	if h.rbLeft == nil {
		return nil, h
	}

	if !rbIsRed(h.rbLeft) && !rbIsRed(h.rbLeft.rbLeft) {
		h = rbMoveLeft(h)
	}

	h.rbLeft, n = rbDeleteMin(h.rbLeft)

	return rbFixup(h), n
}
