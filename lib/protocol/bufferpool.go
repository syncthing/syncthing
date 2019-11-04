// Copyright (C) 2016 The Protocol Authors.

package protocol

import "sync"

// Global pool to get buffers from. Requires Blocksizes to be initialised,
// therefore it is initialized in the same init() as BlockSizes
var BufferPool bufferPool

type bufferPool struct {
	pools []sync.Pool
}

func newBufferPool() bufferPool {
	return bufferPool{make([]sync.Pool, len(BlockSizes))}
}

func (p *bufferPool) Get(size int) []byte {
	// Too big, isn't pooled
	if size > MaxBlockSize {
		return make([]byte, size)
	}

	// Find the segment containing a slice guaranteed to be >= size
	var i, blockSize int
	for i, blockSize = range BlockSizes {
		if size <= blockSize {
			break
		}
	}

	var bs []byte
	// Try the fitting and all bigger pools
	for j := i; j < len(BlockSizes); j++ {
		if intf := p.pools[j].Get(); intf != nil {
			bs = *intf.(*[]byte)
			return bs[:size]
		}
	}

	// All pools are empty, must allocate. For very small slices where we
	// didn't have a block to reuse, just allocate a small slice instead of
	// a large one. We won't be able to reuse it, but avoid some overhead.
	if size < MinBlockSize/64 {
		return make([]byte, size)
	}
	return make([]byte, BlockSizes[i])[:size]
}

// Put makes the given byte slice available again in the global pool
func (p *bufferPool) Put(bs []byte) {
	c := cap(bs)
	// Don't buffer huge byte slices or slices that are too small to be
	// safely reused
	if c > MaxBlockSize*1.5 || c < MinBlockSize {
		return
	}
	for i, blockSize := range BlockSizes {
		if c <= blockSize {
			p.pools[i].Put(&bs)
			return
		}
	}
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
