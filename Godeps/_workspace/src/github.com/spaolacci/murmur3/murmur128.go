package murmur3

import (
	//"encoding/binary"
	"hash"
	"unsafe"
)

const (
	c1_128 = 0x87c37b91114253d5
	c2_128 = 0x4cf5ad432745937f
)

// Make sure interfaces are correctly implemented.
var (
	_ hash.Hash = new(digest128)
	_ Hash128   = new(digest128)
	_ bmixer    = new(digest128)
)

// Hack: the standard api doesn't define any Hash128 interface.
type Hash128 interface {
	hash.Hash
	Sum128() (uint64, uint64)
}

// digest128 represents a partial evaluation of a 128 bites hash.
type digest128 struct {
	digest
	h1 uint64 // Unfinalized running hash part 1.
	h2 uint64 // Unfinalized running hash part 2.
}

func New128() Hash128 {
	d := new(digest128)
	d.bmixer = d
	d.Reset()
	return d
}

func (d *digest128) Size() int { return 16 }

func (d *digest128) reset() { d.h1, d.h2 = 0, 0 }

func (d *digest128) Sum(b []byte) []byte {
	h1, h2 := d.Sum128()
	return append(b,
		byte(h1>>56), byte(h1>>48), byte(h1>>40), byte(h1>>32),
		byte(h1>>24), byte(h1>>16), byte(h1>>8), byte(h1),

		byte(h2>>56), byte(h2>>48), byte(h2>>40), byte(h2>>32),
		byte(h2>>24), byte(h2>>16), byte(h2>>8), byte(h2),
	)
}

func (d *digest128) bmix(p []byte) (tail []byte) {
	h1, h2 := d.h1, d.h2

	nblocks := len(p) / 16
	for i := 0; i < nblocks; i++ {
		t := (*[2]uint64)(unsafe.Pointer(&p[i*16]))
		k1, k2 := t[0], t[1]

		k1 *= c1_128
		k1 = (k1 << 31) | (k1 >> 33) // rotl64(k1, 31)
		k1 *= c2_128
		h1 ^= k1

		h1 = (h1 << 27) | (h1 >> 37) // rotl64(h1, 27)
		h1 += h2
		h1 = h1*5 + 0x52dce729

		k2 *= c2_128
		k2 = (k2 << 33) | (k2 >> 31) // rotl64(k2, 33)
		k2 *= c1_128
		h2 ^= k2

		h2 = (h2 << 31) | (h2 >> 33) // rotl64(h2, 31)
		h2 += h1
		h2 = h2*5 + 0x38495ab5
	}
	d.h1, d.h2 = h1, h2
	return p[nblocks*d.Size():]
}

func (d *digest128) Sum128() (h1, h2 uint64) {

	h1, h2 = d.h1, d.h2

	var k1, k2 uint64
	switch len(d.tail) & 15 {
	case 15:
		k2 ^= uint64(d.tail[14]) << 48
		fallthrough
	case 14:
		k2 ^= uint64(d.tail[13]) << 40
		fallthrough
	case 13:
		k2 ^= uint64(d.tail[12]) << 32
		fallthrough
	case 12:
		k2 ^= uint64(d.tail[11]) << 24
		fallthrough
	case 11:
		k2 ^= uint64(d.tail[10]) << 16
		fallthrough
	case 10:
		k2 ^= uint64(d.tail[9]) << 8
		fallthrough
	case 9:
		k2 ^= uint64(d.tail[8]) << 0

		k2 *= c2_128
		k2 = (k2 << 33) | (k2 >> 31) // rotl64(k2, 33)
		k2 *= c1_128
		h2 ^= k2

		fallthrough

	case 8:
		k1 ^= uint64(d.tail[7]) << 56
		fallthrough
	case 7:
		k1 ^= uint64(d.tail[6]) << 48
		fallthrough
	case 6:
		k1 ^= uint64(d.tail[5]) << 40
		fallthrough
	case 5:
		k1 ^= uint64(d.tail[4]) << 32
		fallthrough
	case 4:
		k1 ^= uint64(d.tail[3]) << 24
		fallthrough
	case 3:
		k1 ^= uint64(d.tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint64(d.tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint64(d.tail[0]) << 0
		k1 *= c1_128
		k1 = (k1 << 31) | (k1 >> 33) // rotl64(k1, 31)
		k1 *= c2_128
		h1 ^= k1
	}

	h1 ^= uint64(d.clen)
	h2 ^= uint64(d.clen)

	h1 += h2
	h2 += h1

	h1 = fmix64(h1)
	h2 = fmix64(h2)

	h1 += h2
	h2 += h1

	return h1, h2
}

func fmix64(k uint64) uint64 {
	k ^= k >> 33
	k *= 0xff51afd7ed558ccd
	k ^= k >> 33
	k *= 0xc4ceb9fe1a85ec53
	k ^= k >> 33
	return k
}

/*
func rotl64(x uint64, r byte) uint64 {
	return (x << r) | (x >> (64 - r))
}
*/

// Sum128 returns the MurmurHash3 sum of data. It is equivalent to the
// following sequence (without the extra burden and the extra allocation):
//     hasher := New128()
//     hasher.Write(data)
//     return hasher.Sum128()
func Sum128(data []byte) (h1 uint64, h2 uint64) {
	d := &digest128{h1: 0, h2: 0}
	d.tail = d.bmix(data)
	d.clen = len(data)
	return d.Sum128()
}
