// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Global pool to get buffers from. Initialized in init().
var BufferPool *bufferPool

type bufferPool struct {
	puts   atomic.Int64
	skips  atomic.Int64
	misses atomic.Int64
	pools  []sync.Pool
	hits   []atomic.Int64
}

func newBufferPool() *bufferPool {
	return &bufferPool{
		pools: make([]sync.Pool, len(BlockSizes)),
		hits:  make([]atomic.Int64, len(BlockSizes)),
	}
}

func (p *bufferPool) Get(size int) []byte {
	// Too big, isn't pooled
	if size > MaxBlockSize {
		p.skips.Add(1)
		return make([]byte, size)
	}

	// Try the fitting and all bigger pools
	bkt := getBucketForLen(size)
	for j := bkt; j < len(BlockSizes); j++ {
		if intf := p.pools[j].Get(); intf != nil {
			p.hits[j].Add(1)
			bs := *intf.(*[]byte)
			return bs[:size]
		}
	}

	p.misses.Add(1)

	// All pools are empty, must allocate. For very small slices where we
	// didn't have a block to reuse, just allocate a small slice instead of
	// a large one. We won't be able to reuse it, but avoid some overhead.
	if size < MinBlockSize/64 {
		return make([]byte, size)
	}
	return make([]byte, BlockSizes[bkt])[:size]
}

// Put makes the given byte slice available again in the global pool.
// You must only Put() slices that were returned by Get() or Upgrade().
func (p *bufferPool) Put(bs []byte) {
	// Don't buffer slices outside of our pool range
	if cap(bs) > MaxBlockSize || cap(bs) < MinBlockSize {
		p.skips.Add(1)
		return
	}

	p.puts.Add(1)
	bkt := putBucketForCap(cap(bs))
	p.pools[bkt].Put(&bs)
}

// Upgrade grows the buffer to the requested size, while attempting to reuse
// it if possible.
func (p *bufferPool) Upgrade(bs []byte, size int) []byte {
	if cap(bs) >= size {
		// Reslicing is enough, lets go!
		return bs[:size]
	}

	// It was too small. But it pack into the pool and try to get another
	// buffer.
	p.Put(bs)
	return p.Get(size)
}

// getBucketForLen returns the bucket where we should get a slice of a
// certain length. Each bucket is guaranteed to hold slices that are
// precisely the block size for that bucket, so if the block size is larger
// than our size we are good.
func getBucketForLen(len int) int {
	for i, blockSize := range BlockSizes {
		if len <= blockSize {
			return i
		}
	}

	panic(fmt.Sprintf("bug: tried to get impossible block len %d", len))
}

// putBucketForCap returns the bucket where we should put a slice of a
// certain capacity. Each bucket is guaranteed to hold slices that are
// precisely the block size for that bucket, so we just find the matching
// one.
func putBucketForCap(cap int) int {
	for i, blockSize := range BlockSizes {
		if cap == blockSize {
			return i
		}
	}

	panic(fmt.Sprintf("bug: tried to put impossible block cap %d", cap))
}
