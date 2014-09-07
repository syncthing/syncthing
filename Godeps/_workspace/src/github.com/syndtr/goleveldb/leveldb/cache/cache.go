// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cache provides interface and implementation of a cache algorithms.
package cache

import (
	"sync/atomic"
)

// SetFunc is the function that will be called by Namespace.Get to create
// a cache object, if charge is less than one than the cache object will
// not be registered to cache tree, if value is nil then the cache object
// will not be created.
type SetFunc func() (charge int, value interface{})

// DelFin is the function that will be called as the result of a delete operation.
// Exist == true is indication that the object is exist, and pending == true is
// indication of deletion already happen but haven't done yet (wait for all handles
// to be released). And exist == false means the object doesn't exist.
type DelFin func(exist, pending bool)

// PurgeFin is the function that will be called as the result of a purge operation.
type PurgeFin func(ns, key uint64)

// Cache is a cache tree. A cache instance must be goroutine-safe.
type Cache interface {
	// SetCapacity sets cache tree capacity.
	SetCapacity(capacity int)

	// Capacity returns cache tree capacity.
	Capacity() int

	// Used returns used cache tree capacity.
	Used() int

	// Size returns entire alive cache objects size.
	Size() int

	// NumObjects returns number of alive objects.
	NumObjects() int

	// GetNamespace gets cache namespace with the given id.
	// GetNamespace is never return nil.
	GetNamespace(id uint64) Namespace

	// PurgeNamespace purges cache namespace with the given id from this cache tree.
	// Also read Namespace.Purge.
	PurgeNamespace(id uint64, fin PurgeFin)

	// ZapNamespace detaches cache namespace with the given id from this cache tree.
	// Also read Namespace.Zap.
	ZapNamespace(id uint64)

	// Purge purges all cache namespace from this cache tree.
	// This is behave the same as calling Namespace.Purge method on all cache namespace.
	Purge(fin PurgeFin)

	// Zap detaches all cache namespace from this cache tree.
	// This is behave the same as calling Namespace.Zap method on all cache namespace.
	Zap()
}

// Namespace is a cache namespace. A namespace instance must be goroutine-safe.
type Namespace interface {
	// Get gets cache object with the given key.
	// If cache object is not found and setf is not nil, Get will atomically creates
	// the cache object by calling setf. Otherwise Get will returns nil.
	//
	// The returned cache handle should be released after use by calling Release
	// method.
	Get(key uint64, setf SetFunc) Handle

	// Delete removes cache object with the given key from cache tree.
	// A deleted cache object will be released as soon as all of its handles have
	// been released.
	// Delete only happen once, subsequent delete will consider cache object doesn't
	// exist, even if the cache object ins't released yet.
	//
	// If not nil, fin will be called if the cache object doesn't exist or when
	// finally be released.
	//
	// Delete returns true if such cache object exist and never been deleted.
	Delete(key uint64, fin DelFin) bool

	// Purge removes all cache objects within this namespace from cache tree.
	// This is the same as doing delete on all cache objects.
	//
	// If not nil, fin will be called on all cache objects when its finally be
	// released.
	Purge(fin PurgeFin)

	// Zap detaches namespace from cache tree and release all its cache objects.
	// A zapped namespace can never be filled again.
	// Calling Get on zapped namespace will always return nil.
	Zap()
}

// Handle is a cache handle.
type Handle interface {
	// Release releases this cache handle. This method can be safely called mutiple
	// times.
	Release()

	// Value returns value of this cache handle.
	// Value will returns nil after this cache handle have be released.
	Value() interface{}
}

const (
	DelNotExist = iota
	DelExist
	DelPendig
)

// Namespace state.
type nsState int

const (
	nsEffective nsState = iota
	nsZapped
)

// Node state.
type nodeState int

const (
	nodeZero nodeState = iota
	nodeEffective
	nodeEvicted
	nodeDeleted
)

// Fake handle.
type fakeHandle struct {
	value interface{}
	fin   func()
	once  uint32
}

func (h *fakeHandle) Value() interface{} {
	if atomic.LoadUint32(&h.once) == 0 {
		return h.value
	}
	return nil
}

func (h *fakeHandle) Release() {
	if !atomic.CompareAndSwapUint32(&h.once, 0, 1) {
		return
	}
	if h.fin != nil {
		h.fin()
		h.fin = nil
	}
}
