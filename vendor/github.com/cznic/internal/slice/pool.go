// Copyright 2016 The Internal Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package slice implements pools of pointers to slices.
package slice

import (
	"sync"

	"github.com/cznic/mathutil"
)

var (
	// Bytes is a ready to use *[]byte Pool.
	Bytes *Pool
	// Ints is a ready to use *[]int Pool.
	Ints *Pool
)

func init() {
	Bytes = newBytes()
	Ints = NewPool(
		func(size int) interface{} { // create
			b := make([]int, size)
			return &b
		},
		func(s interface{}) { // clear
			b := *s.(*[]int)
			b = b[:cap(b)]
			for i := range b {
				b[i] = 0
			}
		},
		func(s interface{}, size int) { // setSize
			p := s.(*[]int)
			*p = (*p)[:size]
		},
		func(s interface{}) int { return cap(*s.(*[]int)) }, // cap
	)
}

func newBytes() *Pool {
	return NewPool(
		func(size int) interface{} { // create
			b := make([]byte, size)
			return &b
		},
		func(s interface{}) { // clear
			b := *s.(*[]byte)
			b = b[:cap(b)]
			for i := range b {
				b[i] = 0
			}
		},
		func(s interface{}, size int) { // setSize
			p := s.(*[]byte)
			*p = (*p)[:size]
		},
		func(s interface{}) int { return cap(*s.(*[]byte)) }, // cap
	)
}

// Pool implements a pool of pointers to slices.
//
// Example usage pattern (assuming pool is, for example, a *[]byte Pool)
//
//	p := pool.Get(size).(*[]byte)
//	b := *p	// Now you can use b in any way you need.
//	...
//	// When b will not be used anymore
//	pool.Put(p)
//	...
//	// If b or p are not going out of scope soon, optionally
//	b = nil
//	p = nil
//
// Otherwise the pool cannot release the slice on garbage collection.
//
// Do not do
//
//	p := pool.Get(size).(*[]byte)
//	b := *p
//	...
//	pool.Put(&b)
//
// or
//
//	b := *pool.Get(size).(*[]byte)
//	...
//	pool.Put(&b)
type Pool struct {
	cap     func(interface{}) int
	clear   func(interface{})
	m       [63]sync.Pool
	null    interface{}
	setSize func(interface{}, int)
}

// NewPool returns a newly created Pool. Assuming the desired slice type is
// []T:
//
// The create function returns a *[]T of len == cap == size.
//
// The argument of clear is *[]T and the function sets all the slice elements
// to the respective zero value.
//
// The setSize function gets a *[]T and sets its len to size.
//
// The cap function gets a *[]T and returns its capacity.
func NewPool(
	create func(size int) interface{},
	clear func(interface{}),
	setSize func(p interface{}, size int),
	cap func(p interface{}) int,
) *Pool {
	p := &Pool{clear: clear, setSize: setSize, cap: cap, null: create(0)}
	for i := range p.m {
		size := 1 << uint(i)
		p.m[i] = sync.Pool{New: func() interface{} {
			// 0:     1 -      1
			// 1:    10 -     10
			// 2:    11 -    100
			// 3:   101 -   1000
			// 4:  1001 -  10000
			// 5: 10001 - 100000
			return create(size)
		}}
	}
	return p
}

// CGet returns a *[]T of len size. The pointed to slice is zeroed up to its
// cap. CGet panics for size < 0.
//
// CGet is safe for concurrent use by multiple goroutines.
func (p *Pool) CGet(size int) interface{} {
	s := p.Get(size)
	p.clear(s)
	return s
}

// Get returns a *[]T of len size. The pointed to slice is not zeroed. Get
// panics for size < 0.
//
// Get is safe for concurrent use by multiple goroutines.
func (p *Pool) Get(size int) interface{} {
	var index int
	switch {
	case size < 0:
		panic("Pool.Get: negative size")
	case size == 0:
		return p.null
	case size > 1:
		index = mathutil.Log2Uint64(uint64(size-1)) + 1
	}
	s := p.m[index].Get()
	p.setSize(s, size)
	return s
}

// Put puts a *[]T into a pool for possible later reuse by CGet or Get. Put
// panics is its argument is not of type *[]T.
//
// Put is safe for concurrent use by multiple goroutines.
func (p *Pool) Put(b interface{}) {
	size := p.cap(b)
	if size == 0 {
		return
	}

	p.m[mathutil.Log2Uint64(uint64(size))].Put(b)
}
