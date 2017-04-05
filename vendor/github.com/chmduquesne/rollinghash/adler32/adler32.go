// Package rollinghash/adler32 implements a rolling version of hash/adler32

package adler32

import (
	"hash"
	vanilla "hash/adler32"

	"github.com/chmduquesne/rollinghash"
)

const (
	Mod  = 65521
	Size = 4
)

type digest struct {
	a, b uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
	n      uint32

	vanilla hash.Hash32
}

// Reset resets the Hash to its initial state.
func (d *digest) Reset() {
	d.window = d.window[:0] // Reset the size but don't reallocate
	d.a = 1
	d.b = 0
	d.oldest = 0
}

// New returns a new rollinghash.Hash32 computing the rolling Adler-32
// checksum. The window is copied from the last Write(). This window is
// only used to determine which is the oldest element (leaving the
// window). The calls to Roll() do not recompute the whole checksum.
func New() rollinghash.Hash32 {
	return &digest{
		a:       1,
		b:       0,
		window:  make([]byte, 0),
		oldest:  0,
		vanilla: vanilla.New(),
	}
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
func (d *digest) Write(p []byte) (int, error) {
	// Copy the window, avoiding allocations where possible
	if len(d.window) != len(p) {
		if cap(d.window) >= len(p) {
			d.window = d.window[:len(p)]
		} else {
			d.window = make([]byte, len(p))
		}
	}
	copy(d.window, p)

	// Piggy-back on the core implementation
	d.vanilla.Reset()
	d.vanilla.Write(p)
	s := d.vanilla.Sum32()
	d.a, d.b = s&0xffff, s>>16
	d.n = uint32(len(p)) % Mod
	return len(d.window), nil
}

func (d *digest) Sum32() uint32 {
	return d.b<<16 | d.a
}

func (d *digest) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the leaving byte and the
// entering byte. See
// http://stackoverflow.com/questions/40985080/why-does-my-rolling-adler32-checksum-not-work-in-go-modulo-arithmetic
func (d *digest) Roll(b byte) {
	if len(d.window) == 0 {
		d.window = make([]byte, 1)
		d.window[0] = b
	}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint32(b)
	leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = b
	d.oldest += 1
	if d.oldest >= len(d.window) {
		d.oldest = 0
	}

	// compute
	d.a = (d.a + Mod + enter - leave) % Mod
	d.b = (d.b + (d.n*leave/Mod+1)*Mod + d.a - (d.n * leave) - 1) % Mod
}
