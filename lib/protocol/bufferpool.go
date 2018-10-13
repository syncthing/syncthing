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
	var i int
	for i = range BlockSizes {
		if size <= BlockSizes[i] {
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
	// All pools are empty, must allocate.
	return make([]byte, BlockSizes[i])[:size]
}

// Put makes the given byte slice availabe again in the global pool
func (p *bufferPool) Put(bs []byte) {
	c := cap(bs)
	// Don't buffer huge byte slices
	if c > 2*MaxBlockSize {
		return
	}
	for i := range BlockSizes {
		if c >= BlockSizes[i] {
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
