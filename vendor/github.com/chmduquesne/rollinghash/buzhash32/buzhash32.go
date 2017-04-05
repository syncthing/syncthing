// Package rollinghash/buzhash implements buzhash as described by
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial

package buzhash32

import rollinghash "github.com/chmduquesne/rollinghash"

// 256 random integers generated with a dummy python script
var DefaultHash = [256]uint32{
	0xa5659a00, 0x2dbfda02, 0xac29a407, 0xce942c08, 0x48513609,
	0x325f158, 0xb54e5e13, 0xa9063618, 0xa5793419, 0x554b081a,
	0xe5643dac, 0xfb50e41c, 0x2b31661d, 0x335da61f, 0xe702f7b0,
	0xe31c1424, 0x6dfed825, 0xd30cf628, 0xba626a2a, 0x74b9c22b,
	0xa5d1942d, 0xf364ae2f, 0x70d2e84c, 0x190ad208, 0x92e3b740,
	0xd7e9f435, 0x15763836, 0x930ecab4, 0x641ea65e, 0xc0b2eb0a,
	0x2675e03e, 0x1a24c63f, 0xeddbcbb7, 0x3ea42bb2, 0x815f5849,
	0xa55c284b, 0xbb30964c, 0x6f7acc4e, 0x74538a50, 0x66df9652,
	0x2bae8454, 0xfe9d8055, 0x8c866fd4, 0x82f0a63d, 0x8f26365e,
	0xe66c3460, 0x6423266, 0x60696abc, 0xf75de6d, 0xd20c86e,
	0x69f8c6f, 0x8ac0f470, 0x273aab68, 0x4e044c74, 0xb2ec7875,
	0xf642d676, 0xd719e877, 0xee557e78, 0xdd20be7a, 0xd252707e,
	0xfa507a7f, 0xee537683, 0x6aac7684, 0x340e3485, 0x1c291288,
	0xab89c8c, 0xbe6e6c8d, 0xf99cf2f7, 0x69c65890, 0xd3757491,
	0xfeb63895, 0x67067a96, 0xa0089b19, 0x6c449898, 0x4eca749a,
	0x1101229b, 0x6b86d29d, 0x9c21be9e, 0xc5904933, 0xe1e820a3,
	0x6bd524a6, 0xd4695ea7, 0xc3d007e0, 0xbed8e4a9, 0x1c49d8af,
	0xedbae4b1, 0x1d2af6b4, 0x79526b9, 0xbc1d5abb, 0x6a2eb8bc,
	0x611b3695, 0x745c3cc4, 0x81005276, 0x5f442c8, 0x42dc30ca,
	0x55e460cb, 0x47648cc, 0x20da7122, 0xc4eedccd, 0xc21c14d0,
	0x27b5dfa9, 0x7e961fce, 0x8d0296d6, 0xce3684d7, 0x28e96da,
	0xedf7dcdc, 0x6817a0df, 0x51caae0, 0x8f226e1, 0xa1a00ce3,
	0xf811c6e5, 0x13e96ee6, 0xd4d4e4d1, 0xab160ee9, 0xb2cf06ea,
	0xf4ab6eb, 0x998f56f1, 0x16974cf2, 0xd42438f5, 0xe00ba6f7,
	0xbf01b8f8, 0x7a8a00f9, 0xdded6a7f, 0xb0ce58fd, 0xe5d81901,
	0xcc823b03, 0xc962e704, 0x2b4aff05, 0x5bcb7181, 0xe7207108,
	0xf3c93109, 0x1ffb650a, 0x37a31ad7, 0xfe27322d, 0x15b16d11,
	0x51a70512, 0xb579d92e, 0x53658284, 0x91fedb1b, 0x2ef0b122,
	0x93966523, 0xfa66af26, 0xa7fac32b, 0x7a81692c, 0x4f8d7f2e,
	0xf9875730, 0xa5ab2331, 0x79db8333, 0x8be32937, 0xf900af39,
	0xd09d4f3a, 0x9b22053d, 0xd2053e1c, 0xd0deaa35, 0x4a975740,
	0xcb3706e0, 0x40aea6cd, 0x769fdd44, 0x7e3e4947, 0xc20ac949,
	0x3788c34b, 0x9b23f74c, 0xb33e441d, 0x705d8a8d, 0x6a5e3a84,
	0xb4f955e3, 0xf681a155, 0x7dec1b56, 0x7bf5df58, 0xd3fa255a,
	0x3797c15c, 0xbf511562, 0xb048d65, 0xcd04f367, 0xae3a8368,
	0x769c856d, 0xc7bb9d6f, 0xe43e1f71, 0xa24de03e, 0x7f8cb376,
	0x618b778, 0x19e02f33, 0x2f810eea, 0x2b1ce595, 0x4f2f7180,
	0x72903140, 0x26a44584, 0x6af97e96, 0xb08acb86, 0x4d25cd41,
	0x1d74fd89, 0xe0f5b277, 0xbad158c, 0x5fed3b8d, 0x68b26794,
	0xcbe58795, 0xc1180797, 0xa1352399, 0x71dacd9c, 0x42b5549a,
	0xbf5371a0, 0x7ed41fa1, 0x6fe29a3, 0xa779fba5, 0x48a095a7,
	0xc2cad5a8, 0x7d7f15a9, 0xccd195aa, 0x2a9047ac, 0x3ec66ef2,
	0x252743ae, 0xdd8827af, 0x85fc5055, 0xb9d5c7b2, 0x5a224fb4,
	0xec26e7b6, 0xe4d8f7b7, 0x6e5aa58d, 0xeff753b9, 0x6c391fbb,
	0x989f65bc, 0x2fe4a7c1, 0x9d1d9bc3, 0xa09aadc6, 0x2df33fc8,
	0x5ec27933, 0x5e7f41cb, 0xb920f7cd, 0xc1a603ce, 0xf0888fcf,
	0xdc4ad1d1, 0x34b3dbd4, 0x170981d5, 0x22e5b5d6, 0x13049bd7,
	0xf12a8b95, 0xff7e87d9, 0xabb74b84, 0x215cff4f, 0xaf24f7dc,
	0xc87461d, 0x41a55e0, 0xfde9b9e1, 0x1d1956fb, 0x13d60de4,
	0x435f93e5, 0xe0ab5de6, 0x5c1d3fe7, 0x411a1fe8, 0x55e102a9,
	0x3d9b07eb, 0xdd6b8dee, 0x741293f3, 0xa5b10ca9, 0x5abad5fd,
	0x22372f55,
}

// The size of the checksum.
const Size = 4

// digest represents the partial evaluation of a checksum.
type digest struct {
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
func (d *digest) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
	d.sum = 0
}

func New() rollinghash.Hash32 {
	return NewFromUint32Array(DefaultHash)
}

// NewFromUint32Array returns a buzhash based on the provided table uint32 values.
func NewFromUint32Array(b [256]uint32) rollinghash.Hash32 {
	return &digest{
		sum:      0,
		window:   make([]byte, 0),
		oldest:   0,
		bytehash: b,
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
func (d *digest) Write(data []byte) (int, error) {
	// Copy the window, avoiding allocations where possible
	if len(d.window) != len(data) {
		if cap(d.window) >= len(data) {
			d.window = d.window[:len(data)]
		} else {
			d.window = make([]byte, len(data))
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

func (d *digest) Sum32() uint32 {
	return d.sum
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
