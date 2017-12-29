// Package rollinghash/buzhash implements buzhash as described by
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial

package buzhash32

import (
	"math/rand"

	rollinghash "github.com/chmduquesne/rollinghash"
)

var defaultHashes [256]uint32

func init() {
	defaultHashes = GenerateHashes(1)
}

// The size of the checksum.
const Size = 4

// Buzhash32 is a digest which satisfies the rollinghash.Hash32 interface.
// It implements the cyclic polynomial algorithm
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial
type Buzhash32 struct {
	sum               uint32
	nRotate           uint
	nRotateComplement uint // redundant, but pre-computed to spare an operation

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window   []byte
	oldest   int
	bytehash [256]uint32
}

// Reset resets the Hash to its initial state.
func (d *Buzhash32) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
	d.sum = 0
}

// GenerateHashes generates a list of hashes to use with buzhash
func GenerateHashes(seed int64) (res [256]uint32) {
	random := rand.New(rand.NewSource(seed))
	used := make(map[uint32]bool)
	for i, _ := range res {
		x := uint32(random.Int63())
		for used[x] {
			x = uint32(random.Int63())
		}
		used[x] = true
		res[i] = x
	}
	return res
}

// New returns a buzhash based on a list of hashes provided by a call to
// GenerateHashes, seeded with the default value 1.
func New() *Buzhash32 {
	return NewFromUint32Array(defaultHashes)
}

// NewFromUint32Array returns a buzhash based on the provided table uint32 values.
func NewFromUint32Array(b [256]uint32) *Buzhash32 {
	return &Buzhash32{
		sum:      0,
		window:   make([]byte, 1, rollinghash.DefaultWindowCap),
		oldest:   0,
		bytehash: b,
	}
}

// Size is 4 bytes
func (d *Buzhash32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Buzhash32) BlockSize() int { return 1 }

// Write (re)initializes the rolling window with the input byte slice and
// adds its data to the digest.
func (d *Buzhash32) Write(data []byte) (int, error) {
	// Copy the window, avoiding allocations where possible
	l := len(data)
	if l == 0 {
		l = 1
	}
	if len(d.window) != l {
		if cap(d.window) >= l {
			d.window = d.window[:l]
		} else {
			d.window = make([]byte, l)
		}
	}
	copy(d.window, data)

	for _, c := range d.window {
		d.sum = d.sum<<1 | d.sum>>31
		d.sum ^= d.bytehash[int(c)]
	}
	d.nRotate = uint(len(d.window)) % 32
	d.nRotateComplement = 32 - d.nRotate
	return len(d.window), nil
}

// Sum32 returns the hash as a uint32
func (d *Buzhash32) Sum32() uint32 {
	return d.sum
}

// Sum returns the hash as byte slice
func (d *Buzhash32) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Buzhash32) Roll(c byte) {
	// extract the entering/leaving bytes and update the circular buffer.
	hn := d.bytehash[int(c)]
	h0 := d.bytehash[int(d.window[d.oldest])]

	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.sum = (d.sum<<1 | d.sum>>31) ^ (h0<<d.nRotate | h0>>d.nRotateComplement) ^ hn
}
