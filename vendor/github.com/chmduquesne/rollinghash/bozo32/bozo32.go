// Package rollinghash/bozo32 is a wrong implementation of the rabinkarp
// checksum. In practice, it works very well and exhibits all the
// properties wanted from a rolling checksum, so after realising that this
// code did not implement the rabinkarp checksum as described in the
// original paper, it was renamed from rabinkarp32 to bozo32 and kept
// in this package.

package bozo32

import rollinghash "github.com/chmduquesne/rollinghash"

// The size of the checksum.
const Size = 4

// Bozo32 is a digest which satisfies the rollinghash.Hash32 interface.
type Bozo32 struct {
	a       uint32
	h       uint32
	aPowerN uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *Bozo32) Reset() {
	d.h = 0
	d.aPowerN = 1
	d.window = nil
	d.oldest = 0
}

func NewFromInt(a uint32) *Bozo32 {
	return &Bozo32{
		a:       a,
		h:       0,
		aPowerN: 1,
		window:  make([]byte, 1, rollinghash.DefaultWindowCap),
		oldest:  0,
	}
}

func New() *Bozo32 {
	return NewFromInt(65521) // largest prime fitting in 16 bits
}

// Size is 4 bytes
func (d *Bozo32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Bozo32) BlockSize() int { return 1 }

// Write (re)initializes the rolling window with the input byte slice and
// adds its data to the digest. It never returns an error.
func (d *Bozo32) Write(data []byte) (int, error) {
	// Copy the window
	l := len(data)
	if l == 0 {
		l = 1
	}
	if len(d.window) >= l {
		d.window = d.window[:l]
	} else {
		d.window = make([]byte, l)
	}
	copy(d.window, data)

	for _, c := range d.window {
		d.h *= d.a
		d.h += uint32(c)
		d.aPowerN *= d.a
	}
	return len(d.window), nil
}

// Sum32 returns the hash as a uint32
func (d *Bozo32) Sum32() uint32 {
	return d.h
}

// Sum returns the hash as byte slice
func (d *Bozo32) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Bozo32) Roll(c byte) {
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint32(c)
	leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.h = d.h*d.a + enter - leave*d.aPowerN
}
