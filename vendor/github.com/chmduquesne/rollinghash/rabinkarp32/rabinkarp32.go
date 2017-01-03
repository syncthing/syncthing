// Package rollinghash/rabinkarp32 implements a particular case of
// rabin-karp where the modulus is 0xffffffff (32 bits of '1')

package rabinkarp32

import rollinghash "github.com/chmduquesne/rollinghash"

// The size of a rabinkarp32 checksum.
const Size = 4

// digest represents the partial evaluation of a checksum.
type digest struct {
	a       uint32
	h       uint32
	aPowerN uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *digest) Reset() {
	d.h = 0
	d.aPowerN = 1
	d.window = nil
	d.oldest = 0
}

func NewFromInt(a uint32) rollinghash.Hash32 {
	return &digest{a: a, h: 0, aPowerN: 1, window: nil, oldest: 0}
}

func New() rollinghash.Hash32 {
	return NewFromInt(65521) // largest prime fitting in 16 bits
}

// Size returns the number of bytes Sum will return.
func (d *digest) Size() int { return Size }

// BlockSize returns the hash's underlying block size.
// The Write method must be able to accept any amount
// of data, but it may operate more efficiently if all
// writes are a multiple of the block size.
func (d *digest) BlockSize() int { return 1 }

// Write (via the embedded io.Writer interface) adds more data to the
// running hash. It never returns an error.
func (d *digest) Write(data []byte) (int, error) {
	// Copy the window
	d.window = make([]byte, len(data))
	copy(d.window, data)
	for _, c := range d.window {
		d.h *= d.a
		d.h += uint32(c)
		d.aPowerN *= d.a
	}
	return len(d.window), nil
}

func (d *digest) Sum32() uint32 {
	return d.h
}

func (d *digest) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the leaving byte and the
// entering byte.
func (d *digest) Roll(c byte) {
	if len(d.window) == 0 {
		d.window = make([]byte, 1)
		d.window[0] = c
	}
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
