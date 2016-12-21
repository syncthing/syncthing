// Copyright (C) 2016 The Protocol Authors.

package protocol

import "sync"

type bufferPool struct {
	minSize int
	pool    sync.Pool
}

// get returns a new buffer of the requested size
func (p *bufferPool) get(size int) []byte {
	intf := p.pool.Get()
	if intf == nil {
		// Pool is empty, must allocate.
		return p.new(size)
	}

	bs := *intf.(*[]byte)
	if cap(bs) < size {
		// Buffer was too small, leave it for someone else and allocate.
		p.pool.Put(intf)
		return p.new(size)
	}

	return bs[:size]
}

// upgrade grows the buffer to the requested size, while attempting to reuse
// it if possible.
func (p *bufferPool) upgrade(bs []byte, size int) []byte {
	if cap(bs) >= size {
		// Reslicing is enough, lets go!
		return bs[:size]
	}

	// It was too small. But it pack into the pool and try to get another
	// buffer.
	p.put(bs)
	return p.get(size)
}

// put returns the buffer to the pool
func (p *bufferPool) put(bs []byte) {
	p.pool.Put(&bs)
}

// new creates a new buffer of the requested size, taking the minimum
// allocation count into account. For internal use only.
func (p *bufferPool) new(size int) []byte {
	allocSize := size
	if allocSize < p.minSize {
		// Avoid allocating tiny buffers that we won't be able to reuse for
		// anything useful.
		allocSize = p.minSize
	}
	return make([]byte, allocSize)[:size]
}
