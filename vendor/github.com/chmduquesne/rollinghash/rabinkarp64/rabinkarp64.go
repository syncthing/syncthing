// Copyright (c) 2014, Alexander Neumann <alexander@bumpern.de>
// Copyright (c) 2017, Christophe-Marie Duquesne <chmd@chmd.fr>
//
// This file was adapted from restic https://github.com/restic/chunker
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package rabinkarp64

import (
	"sync"

	"github.com/chmduquesne/rollinghash"
)

const Size = 8

type tables struct {
	out [256]Pol
	mod [256]Pol
}

// tables are cacheable for a given pol and windowsize
type index struct {
	pol        Pol
	windowsize int
}

type RabinKarp64 struct {
	pol      Pol
	tables   *tables
	polShift uint
	value    Pol

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// cache precomputed tables, these are read-only anyway
var cache struct {
	// For a given polynom and a given window size, we get a table
	entries map[index]*tables
	sync.Mutex
}

func init() {
	cache.entries = make(map[index]*tables)
}

func (d *RabinKarp64) updateTables() {
	windowsize := len(d.window)
	pol := d.pol

	idx := index{d.pol, windowsize}

	cache.Lock()
	t, ok := cache.entries[idx]
	cache.Unlock()
	if ok {
		d.tables = t
		return
	}

	d.tables = buildTables(pol, windowsize)
	cache.Lock()
	cache.entries[idx] = d.tables
	cache.Unlock()
	return
}

func buildTables(pol Pol, windowsize int) (t *tables) {
	t = &tables{}
	// calculate table for sliding out bytes. The byte to slide out is used as
	// the index for the table, the value contains the following:
	// out_table[b] = Hash(b || 0 ||        ...        || 0)
	//                          \ windowsize-1 zero bytes /
	// To slide out byte b_0 for window size w with known hash
	// H := H(b_0 || ... || b_w), it is sufficient to add out_table[b_0]:
	//    H(b_0 || ... || b_w) + H(b_0 || 0 || ... || 0)
	//  = H(b_0 + b_0 || b_1 + 0 || ... || b_w + 0)
	//  = H(    0     || b_1 || ...     || b_w)
	//
	// Afterwards a new byte can be shifted in.
	for b := 0; b < 256; b++ {
		var h Pol
		h <<= 8
		h |= Pol(b)
		h = h.Mod(pol)
		for i := 0; i < windowsize-1; i++ {
			h <<= 8
			h |= Pol(0)
			h = h.Mod(pol)
		}
		t.out[b] = h
	}

	// calculate table for reduction mod Polynomial
	k := pol.Deg()
	for b := 0; b < 256; b++ {
		// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
		//
		// The 8 bits above deg(Polynomial) determine what happens next and so
		// these bits are used as a lookup to this table. The value is split in
		// two parts: Part A contains the result of the modulus operation, part
		// B is used to cancel out the 8 top bits so that one XOR operation is
		// enough to reduce modulo Polynomial
		t.mod[b] = Pol(uint64(b)<<uint(k)).Mod(pol) | (Pol(b) << uint(k))
	}

	return t
}

// NewFromPol returns a RabinKarp64 digest from a polynomial over GF(2).
// It is assumed that the input polynomial is irreducible. You can obtain
// such a polynomial using the RandomPolynomial function.
func NewFromPol(p Pol) *RabinKarp64 {
	res := &RabinKarp64{
		pol:      p,
		tables:   nil,
		polShift: uint(p.Deg() - 8),
		value:    0,
		window:   make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest:   0,
	}
	res.updateTables()
	return res
}

// New returns a RabinKarp64 digest from the default polynomial obtained
// when using RandomPolynomial with the seed 1.
func New() *RabinKarp64 {
	p, err := RandomPolynomial(1)
	if err != nil {
		panic(err)
	}
	return NewFromPol(p)
}

// Reset resets the running hash to its initial state
func (d *RabinKarp64) Reset() {
	d.tables = nil
	d.value = 0
	d.window = d.window[:0]
	d.oldest = 0
	d.updateTables()
}

// Size is 8 bytes
func (d *RabinKarp64) Size() int { return Size }

// BlockSize is 1 byte
func (d *RabinKarp64) BlockSize() int { return 1 }

// Write appends data to the rolling window and updates the digest.
func (d *RabinKarp64) Write(data []byte) (int, error) {
	l := len(data)
	if l == 0 {
		return 0, nil
	}
	// Re-arrange the window so that the leftmost element is at index 0
	n := len(d.window)
	if d.oldest != 0 {
		tmp := make([]byte, d.oldest)
		copy(tmp, d.window[:d.oldest])
		copy(d.window, d.window[d.oldest:])
		copy(d.window[n-d.oldest:], tmp)
		d.oldest = 0
	}
	d.window = append(d.window, data...)

	d.value = 0
	for _, b := range d.window {
		d.value <<= 8
		d.value |= Pol(b)
		d.value = d.value.Mod(d.pol)
	}

	d.updateTables()

	return len(data), nil
}

// Sum64 returns the hash as a uint64
func (d *RabinKarp64) Sum64() uint64 {
	return uint64(d.value)
}

// Sum returns the hash as byte slice
func (d *RabinKarp64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *RabinKarp64) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := c
	leave := uint64(d.window[d.oldest])
	d.window[d.oldest] = c
	d.oldest += 1
	if d.oldest >= len(d.window) {
		d.oldest = 0
	}

	d.value ^= d.tables.out[leave]
	index := byte(d.value >> d.polShift)
	d.value <<= 8
	d.value |= Pol(enter)
	d.value ^= d.tables.mod[index]
}
