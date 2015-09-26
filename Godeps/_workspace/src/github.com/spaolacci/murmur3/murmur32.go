package murmur3

// http://code.google.com/p/guava-libraries/source/browse/guava/src/com/google/common/hash/Murmur3_32HashFunction.java

import (
	"hash"
	"unsafe"
)

// Make sure interfaces are correctly implemented.
var (
	_ hash.Hash   = new(digest32)
	_ hash.Hash32 = new(digest32)
)

const (
	c1_32 uint32 = 0xcc9e2d51
	c2_32 uint32 = 0x1b873593
)

// digest32 represents a partial evaluation of a 32 bites hash.
type digest32 struct {
	digest
	h1 uint32 // Unfinalized running hash.
}

func New32() hash.Hash32 {
	d := new(digest32)
	d.bmixer = d
	d.Reset()
	return d
}

func (d *digest32) Size() int { return 4 }

func (d *digest32) reset() { d.h1 = 0 }

func (d *digest32) Sum(b []byte) []byte {
	h := d.Sum32()
	return append(b, byte(h>>24), byte(h>>16), byte(h>>8), byte(h))
}

// Digest as many blocks as possible.
func (d *digest32) bmix(p []byte) (tail []byte) {
	h1 := d.h1

	nblocks := len(p) / 4
	for i := 0; i < nblocks; i++ {
		k1 := *(*uint32)(unsafe.Pointer(&p[i*4]))

		k1 *= c1_32
		k1 = (k1 << 15) | (k1 >> 17) // rotl32(k1, 15)
		k1 *= c2_32

		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> 19) // rotl32(h1, 13)
		h1 = h1*5 + 0xe6546b64
	}
	d.h1 = h1
	return p[nblocks*d.Size():]
}

func (d *digest32) Sum32() (h1 uint32) {

	h1 = d.h1

	var k1 uint32
	switch len(d.tail) & 3 {
	case 3:
		k1 ^= uint32(d.tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(d.tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(d.tail[0])
		k1 *= c1_32
		k1 = (k1 << 15) | (k1 >> 17) // rotl32(k1, 15)
		k1 *= c2_32
		h1 ^= k1
	}

	h1 ^= uint32(d.clen)

	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return h1
}

/*
func rotl32(x uint32, r byte) uint32 {
	return (x << r) | (x >> (32 - r))
}
*/

// Sum32 returns the MurmurHash3 sum of data. It is equivalent to the
// following sequence (without the extra burden and the extra allocation):
//     hasher := New32()
//     hasher.Write(data)
//     return hasher.Sum32()
func Sum32(data []byte) uint32 {

	var h1 uint32 = 0

	nblocks := len(data) / 4
	var p uintptr
	if len(data) > 0 {
		p = uintptr(unsafe.Pointer(&data[0]))
	}
	p1 := p + uintptr(4*nblocks)
	for ; p < p1; p += 4 {
		k1 := *(*uint32)(unsafe.Pointer(p))

		k1 *= c1_32
		k1 = (k1 << 15) | (k1 >> 17) // rotl32(k1, 15)
		k1 *= c2_32

		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> 19) // rotl32(h1, 13)
		h1 = h1*5 + 0xe6546b64
	}

	tail := data[nblocks*4:]

	var k1 uint32
	switch len(tail) & 3 {
	case 3:
		k1 ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(tail[0])
		k1 *= c1_32
		k1 = (k1 << 15) | (k1 >> 17) // rotl32(k1, 15)
		k1 *= c2_32
		h1 ^= k1
	}

	h1 ^= uint32(len(data))

	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return h1
}
