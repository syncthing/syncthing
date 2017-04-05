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
	"io"
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

// Bytes is similar to bytes.Buffer but may generate less garbage when properly
// Closed. Zero value is ready to use.
type Bytes struct {
	p *[]byte
}

// Bytes return the content of b. The result is R/O.
func (b *Bytes) Bytes() []byte {
	if b.p != nil {
		return *b.p
	}

	return nil
}

// Close will recycle the underlying storage, if any. After Close, b is again
// the zero value.
func (b *Bytes) Close() error {
	if b.p != nil {
		Put(b.p)
		b.p = nil
	}
	return nil
}

// Len returns the size of content in b.
func (b *Bytes) Len() int {
	if b.p != nil {
		return len(*b.p)
	}

	return 0
}

// Reset discard the content of Bytes while keeping the internal storage, if any.
func (b *Bytes) Reset() {
	if b.p != nil {
		*b.p = (*b.p)[:0]
	}
}

// Write writes p into b and returns (len(p), nil).
func (b *Bytes) Write(p []byte) (int, error) {
	n := b.Len()
	b.grow(n + len(p))
	copy((*b.p)[n:], p)
	return len(p), nil
}

// WriteByte writes p into b and returns nil.
func (b *Bytes) WriteByte(p byte) error {
	n := b.Len()
	b.grow(n + 1)
	(*b.p)[n] = p
	return nil
}

// WriteTo writes b's content to w and returns the number of bytes written to w
// and an error, if any.
func (b *Bytes) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(b.Bytes())
	return int64(n), err
}

// WriteString writes s to b and returns (len(s), nil).
func (b *Bytes) WriteString(s string) (int, error) {
	n := b.Len()
	b.grow(n + len(s))
	copy((*b.p)[n:], s)
	return len(s), nil
}

func (b *Bytes) grow(n int) {
	if b.p != nil {
		if n <= cap(*b.p) {
			*b.p = (*b.p)[:n]
			return
		}

		np := Get(2 * n)
		*np = (*np)[:n]
		copy(*np, *b.p)
		Put(b.p)
		b.p = np
		return
	}

	b.p = Get(n)
}
