// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/bits"
)

// A bloomfilter for storing SHA-256 hashes.
//
// The SHA-256 is split into eight 32-bit chunks which are then treated as
// independent hashes.
type bloomfilter struct {
	bits    []uint64
	nhashes int
}

// newBloomfilter constructs a Bloom filter with capacity for at least n
// elements with false positive probability p.
func newBloomfilter(n int, p float64) bloomfilter {
	if p <= 0 || p >= 1 {
		panic("false positive rate for a Bloom filter must be > 0, < 1")
	}

	// Optimal number of hashes for p, capped at the number of 32-bit
	// blocks in a SHA-256, which is eight.
	const nhashesMax = sha256.Size / 4
	nhashes := math.Round(-math.Log2(p))

	if nhashes < 1 {
		nhashes = 1
	} else if nhashes > nhashesMax {
		nhashes = nhashesMax
	}

	// The theoretically optimal nbits/n = -ln(p)/ln(2).
	// We round nbits to the next power of two, so we don't have compute
	// modulos, but stop at 2^32, since we have 32-bit hash functions.
	nbits := uint64(math.Ceil(-(1/math.Ln2)*math.Log2(p)) * float64(n))

	shift := 64 - bits.LeadingZeros64(nbits)
	if shift > 32 {
		shift = 32
	}
	nbits = 1 << shift

	return bloomfilter{
		bits:    make([]uint64, nbits/64),
		nhashes: int(nhashes),
	}
}

func (bf *bloomfilter) Add(sha []byte) {
	_ = sha[4*bf.nhashes-1] // Suppress bounds checks.

	mask := bf.nbits() - 1
	for i := 0; i < bf.nhashes; i++ {
		h := binary.BigEndian.Uint32(sha[:4])
		bf.setbit(h & mask)
		sha = sha[4:]
	}
}

func (bf *bloomfilter) setbit(i uint32) {
	bf.bits[i/64] |= 1 << (i & 63)
}

func (bf *bloomfilter) Test(sha []byte) bool {
	_ = sha[4*bf.nhashes-1] // Suppress bounds checks.

	mask := bf.nbits() - 1
	for i := 0; i < bf.nhashes; i++ {
		h := binary.BigEndian.Uint32(sha[:4])
		if !bf.getbit(h & mask) {
			return false
		}
		sha = sha[4:]
	}
	return true
}

func (bf *bloomfilter) getbit(i uint32) bool {
	x := bf.bits[i/64] & (1 << (i & 63))
	return x != 0
}

func (bf *bloomfilter) nbits() uint32 {
	return uint32(64 * len(bf.bits))
}
