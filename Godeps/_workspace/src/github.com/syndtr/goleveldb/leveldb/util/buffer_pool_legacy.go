// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !go1.3

package util

import (
	"fmt"
	"sync/atomic"
)

type buffer struct {
	b    []byte
	miss int
}

// BufferPool is a 'buffer pool'.
type BufferPool struct {
	pool      [4]chan []byte
	size      [3]uint32
	sizeMiss  [3]uint32
	baseline0 int
	baseline1 int
	baseline2 int

	less    uint32
	equal   uint32
	greater uint32
	miss    uint32
}

func (p *BufferPool) poolNum(n int) int {
	switch {
	case n <= p.baseline0:
		return 0
	case n <= p.baseline1:
		return 1
	case n <= p.baseline2:
		return 2
	default:
		return 3
	}
}

// Get returns buffer with length of n.
func (p *BufferPool) Get(n int) []byte {
	poolNum := p.poolNum(n)
	pool := p.pool[poolNum]
	if poolNum == 0 {
		// Fast path.
		select {
		case b := <-pool:
			switch {
			case cap(b) > n:
				atomic.AddUint32(&p.less, 1)
				return b[:n]
			case cap(b) == n:
				atomic.AddUint32(&p.equal, 1)
				return b[:n]
			default:
				panic("not reached")
			}
		default:
			atomic.AddUint32(&p.miss, 1)
		}

		return make([]byte, n, p.baseline0)
	} else {
		sizePtr := &p.size[poolNum-1]

		select {
		case b := <-pool:
			switch {
			case cap(b) > n:
				atomic.AddUint32(&p.less, 1)
				return b[:n]
			case cap(b) == n:
				atomic.AddUint32(&p.equal, 1)
				return b[:n]
			default:
				atomic.AddUint32(&p.greater, 1)
				if uint32(cap(b)) >= atomic.LoadUint32(sizePtr) {
					select {
					case pool <- b:
					default:
					}
				}
			}
		default:
			atomic.AddUint32(&p.miss, 1)
		}

		if size := atomic.LoadUint32(sizePtr); uint32(n) > size {
			if size == 0 {
				atomic.CompareAndSwapUint32(sizePtr, 0, uint32(n))
			} else {
				sizeMissPtr := &p.sizeMiss[poolNum-1]
				if atomic.AddUint32(sizeMissPtr, 1) == 20 {
					atomic.StoreUint32(sizePtr, uint32(n))
					atomic.StoreUint32(sizeMissPtr, 0)
				}
			}
			return make([]byte, n)
		} else {
			return make([]byte, n, size)
		}
	}
}

// Put adds given buffer to the pool.
func (p *BufferPool) Put(b []byte) {
	pool := p.pool[p.poolNum(cap(b))]
	select {
	case pool <- b:
	default:
	}

}

func (p *BufferPool) String() string {
	return fmt.Sprintf("BufferPool{B·%d Z·%v Zm·%v L·%d E·%d G·%d M·%d}",
		p.baseline0, p.size, p.sizeMiss, p.less, p.equal, p.greater, p.miss)
}

// NewBufferPool creates a new initialized 'buffer pool'.
func NewBufferPool(baseline int) *BufferPool {
	if baseline <= 0 {
		panic("baseline can't be <= 0")
	}
	p := &BufferPool{
		baseline0: baseline,
		baseline1: baseline * 2,
		baseline2: baseline * 4,
	}
	for i, cap := range []int{6, 6, 3, 1} {
		p.pool[i] = make(chan []byte, cap)
	}
	return p
}
