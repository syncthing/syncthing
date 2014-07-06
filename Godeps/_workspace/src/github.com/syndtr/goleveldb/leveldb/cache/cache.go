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

// SetFunc used by Namespace.Get method to create a cache object. SetFunc
// may return ok false, in that case the cache object will not be created.
type SetFunc func() (ok bool, value interface{}, charge int, fin SetFin)

// SetFin will be called when corresponding cache object are released.
type SetFin func()

// DelFin will be called when corresponding cache object are released.
// DelFin will be called after SetFin. The exist is true if the corresponding
// cache object is actually exist in the cache tree.
type DelFin func(exist bool)

// PurgeFin will be called when corresponding cache object are released.
// PurgeFin will be called after SetFin. If PurgeFin present DelFin will
// not be executed but passed to the PurgeFin, it is up to the caller
// to call it or not.
type PurgeFin func(ns, key uint64, delfin DelFin)

// Cache is a cache tree.
type Cache interface {
	// SetCapacity sets cache capacity.
	SetCapacity(capacity int)

	// GetNamespace gets or creates a cache namespace for the given id.
	GetNamespace(id uint64) Namespace

	// Purge purges all cache namespaces, read Namespace.Purge method documentation.
	Purge(fin PurgeFin)

	// Zap zaps all cache namespaces, read Namespace.Zap method documentation.
	Zap(closed bool)
}

// Namespace is a cache namespace.
type Namespace interface {
	// Get gets cache object for the given key. The given SetFunc (if not nil) will
	// be called if the given key does not exist.
	// If the given key does not exist, SetFunc is nil or SetFunc return ok false, Get
	// will return ok false.
	Get(key uint64, setf SetFunc) (obj Object, ok bool)

	// Get deletes cache object for the given key. If exist the cache object will
	// be deleted later when all of its handles have been released (i.e. no one use
	// it anymore) and the given DelFin (if not nil) will finally be  executed. If
	// such cache object does not exist the given DelFin will be executed anyway.
	//
	// Delete returns true if such cache object exist.
	Delete(key uint64, fin DelFin) bool

	// Purge deletes all cache objects, read Delete method documentation.
	Purge(fin PurgeFin)

	// Zap detaches the namespace from the cache tree and delete all its cache
	// objects. The cache objects deletion and finalizers execution are happen
	// immediately, even if its existing handles haven't yet been released.
	// A zapped namespace can't never be filled again.
	// If closed is false then the Get function will always call the given SetFunc
	// if it is not nil, but resultant of the SetFunc will not be cached.
	Zap(closed bool)
}

// Object is a cache object.
type Object interface {
	// Release releases the cache object. Other methods should not be called
	// after the cache object has been released.
	Release()

	// Value returns value of the cache object.
	Value() interface{}
}

// Namespace state.
type nsState int

const (
	nsEffective nsState = iota
	nsZapped
	nsClosed
)

// Node state.
type nodeState int

const (
	nodeEffective nodeState = iota
	nodeEvicted
	nodeRemoved
)

// Fake object.
type fakeObject struct {
	value interface{}
	fin   func()
	once  uint32
}

func (o *fakeObject) Value() interface{} {
	if atomic.LoadUint32(&o.once) == 0 {
		return o.value
	}
	return nil
}

func (o *fakeObject) Release() {
	if !atomic.CompareAndSwapUint32(&o.once, 0, 1) {
		return
	}
	if o.fin != nil {
		o.fin()
		o.fin = nil
	}
}
