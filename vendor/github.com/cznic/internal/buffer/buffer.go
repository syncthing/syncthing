// Copyright 2016 The Internal Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package buffer implements a pool of pointers to byte slices.
//
// Example usage pattern
//
//	p := buffer.Get(size)
//	b := *p	// Now you can use b in any way you need.
//	...
//	// When b will not be used anymore
//	buffer.Put(p)
//	...
//	// If b or p are not going out of scope soon, optionally
//	b = nil
//	p = nil
//
// Otherwise the pool cannot release the buffer on garbage collection.
//
// Do not do
//
//	p := buffer.Get(size)
//	b := *p
//	...
//	buffer.Put(&b)
//
// or
//
//	b := *buffer.Get(size)
//	...
//	buffer.Put(&b)
package buffer

import (
	"github.com/cznic/internal/slice"
)

// CGet returns a pointer to a byte slice of len size. The pointed to byte
// slice is zeroed up to its cap. CGet panics for size < 0.
//
// CGet is safe for concurrent use by multiple goroutines.
func CGet(size int) *[]byte { return slice.Bytes.CGet(size).(*[]byte) }

// Get returns a pointer to a byte slice of len size. The pointed to byte slice
// is not zeroed. Get panics for size < 0.
//
// Get is safe for concurrent use by multiple goroutines.
func Get(size int) *[]byte { return slice.Bytes.Get(size).(*[]byte) }

// Put puts a pointer to a byte slice into a pool for possible later reuse by
// CGet or Get.
//
// Put is safe for concurrent use by multiple goroutines.
func Put(p *[]byte) { slice.Bytes.Put(p) }
