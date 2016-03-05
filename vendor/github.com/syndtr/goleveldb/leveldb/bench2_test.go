// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !go1.2

package leveldb

import (
	"sync/atomic"
	"testing"
)

func BenchmarkDBReadConcurrent(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.gc()
	defer p.close()

	b.ResetTimer()
	b.SetBytes(116)

	b.RunParallel(func(pb *testing.PB) {
		iter := p.newIter()
		defer iter.Release()
		for pb.Next() && iter.Next() {
		}
	})
}

func BenchmarkDBReadConcurrent2(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.gc()
	defer p.close()

	b.ResetTimer()
	b.SetBytes(116)

	var dir uint32
	b.RunParallel(func(pb *testing.PB) {
		iter := p.newIter()
		defer iter.Release()
		if atomic.AddUint32(&dir, 1)%2 == 0 {
			for pb.Next() && iter.Next() {
			}
		} else {
			if pb.Next() && iter.Last() {
				for pb.Next() && iter.Prev() {
				}
			}
		}
	})
}
