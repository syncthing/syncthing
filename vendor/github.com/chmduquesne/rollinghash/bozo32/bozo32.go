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
	a     uint32
	aⁿ    uint32
	value uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *Bozo32) Reset() {
	d.value = 0
	d.aⁿ = 1
	d.oldest = 0
	d.window = d.window[:0]
}

func NewFromInt(a uint32) *Bozo32 {
	return &Bozo32{
		a:      a,
		value:  0,
		aⁿ:     1,
		window: make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest: 0,
	}
}

func New() *Bozo32 {
	return NewFromInt(65521) // largest prime fitting in 16 bits
}

// Size is 4 bytes
func (d *Bozo32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Bozo32) BlockSize() int { return 1 }

// Write appends data to the rolling window and updates the digest. It
// never returns an error.
func (d *Bozo32) Write(data []byte) (int, error) {
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
	d.aⁿ = 1
	for _, c := range d.window {
		d.value *= d.a
		d.value += uint32(c)
		d.aⁿ *= d.a
	}
	return len(data), nil
}

// Sum32 returns the hash as a uint32
func (d *Bozo32) Sum32() uint32 {
	return d.value
}

// Sum returns the hash as byte slice
func (d *Bozo32) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Bozo32) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint32(c)
	leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.value = d.value*d.a + enter - leave*d.aⁿ
}
