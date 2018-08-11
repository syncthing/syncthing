package mmh3

import (
	"encoding/binary"
	"reflect"
	"unsafe"
)

const (
	h32c1 uint32 = 0xcc9e2d51
	h32c2 uint32 = 0x1b873593
	h64c1 uint64 = 0x87c37b91114253d5
	h64c2 uint64 = 0x4cf5ad432745937f
)

func Hash32(key []byte) uint32 {
	length := len(key)
	if length == 0 {
		return 0
	}
	nblocks := length / 4
	var h, k uint32
	for i := 0; i < nblocks; i++ {
		k = binary.LittleEndian.Uint32(key[i*4:])
		k *= h32c1
		k = (k << 15) | (k >> (32 - 15))
		k *= h32c2
		h ^= k
		h = (h << 13) | (h >> (32 - 13))
		h = (h * 5) + 0xe6546b64
	}
	k = 0
	tailIndex := nblocks * 4
	switch length & 3 {
	case 3:
		k ^= uint32(key[tailIndex+2]) << 16
		fallthrough
	case 2:
		k ^= uint32(key[tailIndex+1]) << 8
		fallthrough
	case 1:
		k ^= uint32(key[tailIndex])
		k *= h32c1
		k = (k << 15) | (k >> (32 - 15))
		k *= h32c2
		h ^= k
	}
	h ^= uint32(length)
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

func Hash128(key []byte) []byte {
	length := len(key)
	ret := make([]byte, 16)
	if length == 0 {
		return ret
	}
	nblocks := length / 16
	var h1, h2, k1, k2 uint64
	for i := 0; i < nblocks; i++ {
		k1 = binary.LittleEndian.Uint64(key[i*16:])
		k2 = binary.LittleEndian.Uint64(key[(i*16)+8:])
		k1 *= h64c1
		k1 = (k1 << 31) | (k1 >> (64 - 31))
		k1 *= h64c2
		h1 ^= k1
		h1 = (h1 << 27) | (h1 >> (64 - 27))
		h1 += h2
		h1 = h1*5 + 0x52dce729
		k2 *= h64c2
		k2 = (k2 << 33) | (k2 >> (64 - 33))
		k2 *= h64c1
		h2 ^= k2
		h2 = (h2 << 31) | (h2 >> (64 - 31))
		h2 += h1
		h2 = h2*5 + 0x38495ab5
	}
	k1, k2 = 0, 0
	tailIndex := nblocks * 16
	switch length & 15 {
	case 15:
		k2 ^= uint64(key[tailIndex+14]) << 48
		fallthrough
	case 14:
		k2 ^= uint64(key[tailIndex+13]) << 40
		fallthrough
	case 13:
		k2 ^= uint64(key[tailIndex+12]) << 32
		fallthrough
	case 12:
		k2 ^= uint64(key[tailIndex+11]) << 24
		fallthrough
	case 11:
		k2 ^= uint64(key[tailIndex+10]) << 16
		fallthrough
	case 10:
		k2 ^= uint64(key[tailIndex+9]) << 8
		fallthrough
	case 9:
		k2 ^= uint64(key[tailIndex+8])
		k2 *= h64c2
		k2 = (k2 << 33) | (k2 >> (64 - 33))
		k2 *= h64c1
		h2 ^= k2
		fallthrough
	case 8:
		k1 ^= uint64(key[tailIndex+7]) << 56
		fallthrough
	case 7:
		k1 ^= uint64(key[tailIndex+6]) << 48
		fallthrough
	case 6:
		k1 ^= uint64(key[tailIndex+5]) << 40
		fallthrough
	case 5:
		k1 ^= uint64(key[tailIndex+4]) << 32
		fallthrough
	case 4:
		k1 ^= uint64(key[tailIndex+3]) << 24
		fallthrough
	case 3:
		k1 ^= uint64(key[tailIndex+2]) << 16
		fallthrough
	case 2:
		k1 ^= uint64(key[tailIndex+1]) << 8
		fallthrough
	case 1:
		k1 ^= uint64(key[tailIndex])
		k1 *= h64c1
		k1 = (k1 << 31) | (k1 >> (64 - 31))
		k1 *= h64c2
		h1 ^= k1
	}
	h1 ^= uint64(length)
	h2 ^= uint64(length)
	h1 += h2
	h2 += h1
	h1 ^= h1 >> 33
	h1 *= 0xff51afd7ed558ccd
	h1 ^= h1 >> 33
	h1 *= 0xc4ceb9fe1a85ec53
	h1 ^= h1 >> 33
	h2 ^= h2 >> 33
	h2 *= 0xff51afd7ed558ccd
	h2 ^= h2 >> 33
	h2 *= 0xc4ceb9fe1a85ec53
	h2 ^= h2 >> 33
	h1 += h2
	h2 += h1
	binary.LittleEndian.PutUint64(ret[:], h1)
	binary.LittleEndian.PutUint64(ret[8:], h2)
	return ret
}

// Hash128x64 is a version of MurmurHash which is designed to run only on
// little-endian processors.  It is considerably faster for those processors
// than Hash128.
func Hash128x64(key []byte) []byte {
	length := len(key)
	ret := make([]byte, 16)
	if length == 0 {
		return ret
	}

	rh := *(*reflect.SliceHeader)(unsafe.Pointer(&ret))

	nblocks := length / 16
	var h1, h2, k1, k2 uint64

	h := *(*reflect.SliceHeader)(unsafe.Pointer(&key))
	h.Len = nblocks * 2
	b := *(*[]uint64)(unsafe.Pointer(&h))
	for i := 0; i < len(b); i += 2 {
		k1, k2 = b[i], b[i+1]
		k1 *= h64c1
		k1 = (k1 << 31) | (k1 >> (64 - 31))
		k1 *= h64c2
		h1 ^= k1
		h1 = (h1 << 27) | (h1 >> (64 - 27))
		h1 += h2
		h1 = h1*5 + 0x52dce729
		k2 *= h64c2
		k2 = (k2 << 33) | (k2 >> (64 - 33))
		k2 *= h64c1
		h2 ^= k2
		h2 = (h2 << 31) | (h2 >> (64 - 31))
		h2 += h1
		h2 = h2*5 + 0x38495ab5
	}
	h.Len = length

	k1, k2 = 0, 0
	tailIndex := nblocks * 16
	switch length & 15 {
	case 15:
		k2 ^= uint64(key[tailIndex+14]) << 48
		fallthrough
	case 14:
		k2 ^= uint64(key[tailIndex+13]) << 40
		fallthrough
	case 13:
		k2 ^= uint64(key[tailIndex+12]) << 32
		fallthrough
	case 12:
		k2 ^= uint64(key[tailIndex+11]) << 24
		fallthrough
	case 11:
		k2 ^= uint64(key[tailIndex+10]) << 16
		fallthrough
	case 10:
		k2 ^= uint64(key[tailIndex+9]) << 8
		fallthrough
	case 9:
		k2 ^= uint64(key[tailIndex+8])
		k2 *= h64c2
		k2 = (k2 << 33) | (k2 >> (64 - 33))
		k2 *= h64c1
		h2 ^= k2
		fallthrough
	case 8:
		k1 ^= uint64(key[tailIndex+7]) << 56
		fallthrough
	case 7:
		k1 ^= uint64(key[tailIndex+6]) << 48
		fallthrough
	case 6:
		k1 ^= uint64(key[tailIndex+5]) << 40
		fallthrough
	case 5:
		k1 ^= uint64(key[tailIndex+4]) << 32
		fallthrough
	case 4:
		k1 ^= uint64(key[tailIndex+3]) << 24
		fallthrough
	case 3:
		k1 ^= uint64(key[tailIndex+2]) << 16
		fallthrough
	case 2:
		k1 ^= uint64(key[tailIndex+1]) << 8
		fallthrough
	case 1:
		k1 ^= uint64(key[tailIndex])
		k1 *= h64c1
		k1 = (k1 << 31) | (k1 >> (64 - 31))
		k1 *= h64c2
		h1 ^= k1
	}
	h1 ^= uint64(length)
	h2 ^= uint64(length)
	h1 += h2
	h2 += h1
	h1 ^= h1 >> 33
	h1 *= 0xff51afd7ed558ccd
	h1 ^= h1 >> 33
	h1 *= 0xc4ceb9fe1a85ec53
	h1 ^= h1 >> 33
	h2 ^= h2 >> 33
	h2 *= 0xff51afd7ed558ccd
	h2 ^= h2 >> 33
	h2 *= 0xc4ceb9fe1a85ec53
	h2 ^= h2 >> 33
	h1 += h2
	h2 += h1

	rh.Len = 2
	b = *(*[]uint64)(unsafe.Pointer(&rh))
	b[0] = h1
	b[1] = h2
	rh.Len = 16
	return ret
}

type HashWriter128 struct {
	buf     [16]byte
	h1, h2  uint64
	index   int
	written int64
}

func (hw *HashWriter128) Reset() {
	hw.buf = [16]byte{}
	hw.h1 = 0
	hw.h2 = 0
	hw.index = 0
	hw.written = 0
}

func (hw *HashWriter128) Size() int {
	return 16
}

func (hw *HashWriter128) BlockSize() int {
	return 16
}

func (hw *HashWriter128) Write(b []byte) (int, error) {
	total := 0
	// fill the buffer, then update the internal state
	// of this hash
	for len(b) != 0 {
		n := copy(hw.buf[hw.index:], b)
		total += n
		b = b[n:]
		hw.index += n
		if hw.index != 16 {
			hw.written += int64(total)
			return total, nil
		}
		hw.updateState()
	}
	hw.written += int64(total)
	return total, nil
}

func (hw *HashWriter128) WriteString(s string) (int, error) {
	total := 0
	// fill the buffer, then update the internal state
	// of this hash
	for len(s) != 0 {
		n := copy(hw.buf[hw.index:], s)
		total += n
		s = s[n:]
		hw.index += n
		if hw.index != 16 {
			hw.written += int64(total)
			return total, nil
		}
		hw.updateState()
	}
	hw.written += int64(total)
	return total, nil

}

func (hw *HashWriter128) updateState() {
	hw.index = 0
	k1 := binary.LittleEndian.Uint64(hw.buf[:])
	k2 := binary.LittleEndian.Uint64(hw.buf[8:])
	h1 := hw.h1
	h2 := hw.h2

	k1 *= h64c1
	k1 = (k1 << 31) | (k1 >> (64 - 31))
	k1 *= h64c2
	h1 ^= k1
	h1 = (h1 << 27) | (h1 >> (64 - 27))
	h1 += h2
	h1 = h1*5 + 0x52dce729
	k2 *= h64c2
	k2 = (k2 << 33) | (k2 >> (64 - 33))
	k2 *= h64c1
	h2 ^= k2
	h2 = (h2 << 31) | (h2 >> (64 - 31))
	h2 += h1
	h2 = h2*5 + 0x38495ab5

	hw.h1 = h1
	hw.h2 = h2
}

func (hw *HashWriter128) Sum(b []byte) []byte {
	k1 := uint64(0)
	k2 := uint64(0)
	h1 := hw.h1
	h2 := hw.h2
	switch hw.index {
	case 15:
		k2 ^= uint64(hw.buf[14]) << 48
		fallthrough
	case 14:
		k2 ^= uint64(hw.buf[13]) << 40
		fallthrough
	case 13:
		k2 ^= uint64(hw.buf[12]) << 32
		fallthrough
	case 12:
		k2 ^= uint64(hw.buf[11]) << 24
		fallthrough
	case 11:
		k2 ^= uint64(hw.buf[10]) << 16
		fallthrough
	case 10:
		k2 ^= uint64(hw.buf[9]) << 8
		fallthrough
	case 9:
		k2 ^= uint64(hw.buf[8])
		k2 *= h64c2
		k2 = (k2 << 33) | (k2 >> (64 - 33))
		k2 *= h64c1
		h2 ^= k2
		fallthrough
	case 8:
		k1 ^= uint64(hw.buf[7]) << 56
		fallthrough
	case 7:
		k1 ^= uint64(hw.buf[6]) << 48
		fallthrough
	case 6:
		k1 ^= uint64(hw.buf[5]) << 40
		fallthrough
	case 5:
		k1 ^= uint64(hw.buf[4]) << 32
		fallthrough
	case 4:
		k1 ^= uint64(hw.buf[3]) << 24
		fallthrough
	case 3:
		k1 ^= uint64(hw.buf[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint64(hw.buf[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint64(hw.buf[0])
		k1 *= h64c1
		k1 = (k1 << 31) | (k1 >> (64 - 31))
		k1 *= h64c2
		h1 ^= k1
	}
	h1 ^= uint64(hw.written)
	h2 ^= uint64(hw.written)
	h1 += h2
	h2 += h1
	h1 ^= h1 >> 33
	h1 *= 0xff51afd7ed558ccd
	h1 ^= h1 >> 33
	h1 *= 0xc4ceb9fe1a85ec53
	h1 ^= h1 >> 33
	h2 ^= h2 >> 33
	h2 *= 0xff51afd7ed558ccd
	h2 ^= h2 >> 33
	h2 *= 0xc4ceb9fe1a85ec53
	h2 ^= h2 >> 33
	h1 += h2
	h2 += h1
	var retbuf [8]byte
	binary.LittleEndian.PutUint64(retbuf[:], h1)
	b = append(b, retbuf[:]...)
	binary.LittleEndian.PutUint64(retbuf[:], h2)
	b = append(b, retbuf[:]...)
	return b
}
