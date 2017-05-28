// Package rollinghash/buzhash implements buzhash as described by
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial

package buzhash64

import rollinghash "github.com/chmduquesne/rollinghash"

// 256 random integers generated with a dummy python script
var DefaultHash = [256]uint64{
	0xd6923700885676e1, 0x2ef758a165917c6c, 0xcac8db9a800db08f,
	0x91dfa96019476e5f, 0x61ad4b5c6ec62e4b, 0xbabfc786038a37cb,
	0xb68fe9816c09bb98, 0x6dae71ffcf505baf, 0x8f1d5ac180423f59,
	0x2ddcaf458c114dae, 0x2975abd372acbb39, 0x620f80a1e7fb8ca0,
	0xf8d9b75b40d1fdda, 0x81bff1a297143fab, 0x81935f4d4c31ae6e,
	0xf4e0765a732a3a36, 0x0cded3fd708f0f14, 0xa89cb64087b25da9,
	0xa69372234eb0602d, 0x773a079265484e2d, 0x8dbc0985c9c4e1cb,
	0x000a09a5bc2c80b0, 0xdcaa87a327cead66, 0xd26eaa01fb42ef69,
	0x34411456e2c244d7, 0x1082e6fb20af4bea, 0x1e00897e330f3832,
	0x4253bef8099f370d, 0x890ce98ec0e8a69c, 0x89eb60e611308754,
	0xb39c22caeb5444f1, 0x3e841276d561b022, 0x45292a4e1aaeb117,
	0x1a4b1f1d7aeb46d1, 0x7016fc7d7b3114a6, 0x4fc9ea1dfd505a34,
	0x97b6013b3739d65e, 0x7fcc6abfae8eb598, 0xff8ec196383c66f2,
	0x87ca90161ecaf261, 0xc27ac70e06c9caa3, 0x42c4d7617c362ede,
	0xb38656002f3984f0, 0x0520f83a5be24d68, 0x097cdf0f89aa5ad6,
	0xcc2c65d8ab0e1e32, 0x8c8ebfd12b2c4fa9, 0x9e99c42db2e8be1d,
	0x7bcef376a9003964, 0xbd9bc65dbfebce71, 0xd47a52cea9f0bc02,
	0xeadb465977d2d8ca, 0x43065df5caca1a4b, 0x82f5ae94dd2cc349,
	0xc4e362ab8614dd84, 0xc8922bf4a4bebf05, 0xb1719f57f9a1ed23,
	0xe93a41737e8094ac, 0x33e611a02d4abc93, 0x1dcdb2d07ea310bc,
	0xf7a85d96655b03ef, 0x60aafabd410c3180, 0x18c401b08a67ffeb,
	0xc1eed3417948c90f, 0x525bfe6ad095d998, 0x2a97938c7fd244c2,
	0xbb75ef8569ba728c, 0x53f47ee01b7d1915, 0x51025252faf2890e,
	0xf6bd601ee7ad2608, 0x06a07a64f7afbffa, 0x224f41d09b13aed5,
	0x9f80d30ece1bcd5c, 0x6ce1076c6780de0c, 0xfd123415c8262763,
	0x0d5a643d04d9f438, 0xb92e476b8a36d170, 0x0f533c6c9f196cce,
	0x0071ebbeb03d43af, 0x00dcbdee475f482d, 0x3339362a5b7c099c,
	0x2f957910672cf39e, 0xd69554bbea71bb60, 0x635dd0f5801c9d13,
	0x9832470506cba5cd, 0x77625064508cebba, 0xf428e6bfb38a5d01,
	0x4a086e0cf23ab715, 0xb958fe962ca69576, 0x5d0ab146601ee29f,
	0x90f0042e06fcc096, 0xba69eaa94dd5cbcc, 0xa821915b9a5fa628,
	0xea4f4c03801babed, 0xbc7d5f845d913103, 0xe3cc105d6e4a11ea,
	0x251f29b1422b1af5, 0xd700ffdb510d7634, 0x3002ebfda5cc4592,
	0xf5614fc379a46223, 0x02cb3e88a92ab123, 0x4dab9392f9075ca5,
	0xc8d8c5b39eb3e593, 0x7d6545c168d526df, 0x3cd78f7794445ee4,
	0x24e2a4f47772f09a, 0x43be5ca35c81d4ec, 0x77583ba052e5b605,
	0x92e07779ea9ccd7f, 0xb9dc8617c0a14ea8, 0x8a2821cb56440f77,
	0x15f29e095f8b279e, 0x75c12968e423728c, 0x98cfdf60152b8d2f,
	0x3b5a8db5cf80bd68, 0x2356e64e821e3ac4, 0x320b7aef2daff0d4,
	0xbae4290e875658bf, 0x3b569a663e0b2445, 0xc494ce552c404288,
	0x37a905ddeb550d88, 0x2333bcdc81c0c5c3, 0x8d2682d13259af0c,
	0x5ad34026f7e9b8f4, 0x081970325f7f949d, 0xbcf17bf08e61ef19,
	0xb3e5da3782fd7f03, 0x8ed53c8ec27635e1, 0x79fca624a1e73b7c,
	0xdc9bdb3be0b69b20, 0xc119a348042544cd, 0x1c2408e49ed2a747,
	0xe85f0237669d180d, 0x4508bcebda7465f9, 0x5af245c13d3a8ef7,
	0xbb8bb6b61f021ed0, 0x48eaa45234935f75, 0x2f78f8fb1695eb65,
	0x5dd1e1c8c20a1b76, 0x2f74a22a3159ec45, 0xc64f9c864dfb98cf,
	0xf928618091913d32, 0xec08db6828a11873, 0x029ba990fa5cdba6,
	0x94b870390499d9ba, 0x1086685fce933b2c, 0x6065be1f390c003a,
	0x0f46e9a9d5197803, 0x42833f7327727669, 0xdda6c27eb0d682b3,
	0x5ec3a67f39a77d05, 0x818f5646400a80ec, 0xe45c502c1b655c1b,
	0xd56ddb4fddd63c56, 0x7ebc81bd9fd90fd1, 0x4f6c111625fb5c8e,
	0x6c0fc5f0487dc6ee, 0xc57a12a7159119ed, 0x526bc3b3aadd9dd6,
	0xe89f8367962fe1ea, 0x72bac3c1c99d1845, 0x6f56a75582ae96b9,
	0x7d23f484a9a317f1, 0xe876956fd23c9f95, 0xdd6411629a0dab0a,
	0x827046f4383dad03, 0x36aa4c0e807f9a6d, 0xcfe6ae3f86224a12,
	0x84802ff4baf0e073, 0x19d786fe8a6eecd6, 0x38e9f4a7a4ce611a,
	0x5442a62e65063565, 0x6a6780a6d0257b82, 0x39af9a8cf5786bd7,
	0xe65d071b8fb1c8ee, 0xa63ebe71ad620e4f, 0xdfaaadf4584a0b68,
	0x7bb8f20bd9681981, 0xbfa8bbaae1c5db8b, 0xae3a8b06f286932a,
	0xe92a89eebe1f3292, 0xf11e1c10444edbd2, 0xaf8308bd4915c7f3,
	0x8a1338317833acdc, 0xcec67d8359c7f0e8, 0x3f66a4906e23838a,
	0x9e959f9b1c22fef3, 0x8b5404e71735a246, 0xcbddfc7a87347d03,
	0x7a0d9bd544622f25, 0x3a78e12aab2f532f, 0xddf89b2aecd51922,
	0x38f7465f6d416db4, 0x4349369edbf8ea2a, 0x5e4d38719ad9d621,
	0x0ec281878dddca6a, 0x1c92cae74d6b897a, 0xa0c7c7149a8a76b3,
	0xc469dca35bf1cb2a, 0x6a902e29fcf0ecd4, 0x8c455620d8f5df32,
	0x0b435e9d1c207663, 0x51299e4c5ccbfbd2, 0x365add776bcad536,
	0x957aa2746c2bd41e, 0x414ec15efe36e3a1, 0x6faed19dc4940f61,
	0x6766d7072a6e1d87, 0x3c01b82ebdff7a2d, 0xbbbe879684ec244c,
	0xa425c502184dc5b4, 0x02d77f005bb369ad, 0xb56546c281f8c88f,
	0xb49a866ea16fc9e9, 0x93ee62b3965991ec, 0xf03d0958eb9664a9,
	0x7e57cce4c6c8d5ab, 0x6ae6f4180ea9c5b1, 0xc45fdb113dfba663,
	0x7892fabea1c2d876, 0x7b39106ce2f6d405, 0x12332253ddcff808,
	0x877af9766d5147c4, 0xbbfe3ac2eb6e9d3f, 0xd298d13ac6c3c8c4,
	0x142bc26ad3606528, 0xb0665de1231f2938, 0xf68498ac39f406ec,
	0xc68379a33b570cfe, 0xb43cfe7fcd5d6688, 0x0e18e07f10ee779c,
	0xa021ffa7e745086d, 0xa113db9a2c6bdb43, 0xa00e360382ecd221,
	0x192dc98cbd494a06, 0xb0c9f52cf0252d86, 0x3efb668bcba50726,
	0x114c30f72555d676, 0x99259c3011e85910, 0x5e6c7d80d32133ec,
	0xfa445c39db50cb51, 0x14f1d142aac12947, 0x04dcb1a831c0e97a,
	0x3102eda0466cb1d7, 0xc57ea8effb8c20f5, 0xa3641775b56361af,
	0xaf9608c03cc46398, 0x023b9055ff80b8dc, 0x91965be76eddb8f0,
	0xdcdffd182d67712f, 0xe8bf232ef77feef7, 0x0cc8d45930eb0846,
	0xef2d62d35924c29a, 0x8a68c569490911e2, 0xc44a865ef922d723,
	0xc942fc5e5c343766,
}

// The size of the checksum.
const Size = 8

// digest represents the partial evaluation of a checksum.
type digest struct {
	sum               uint64
	nRotate           uint
	nRotateComplement uint // redundant, but pre-computed to spare an operation

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window   []byte
	oldest   int
	bytehash [256]uint64
}

// Reset resets the Hash to its initial state.
func (d *digest) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
	d.sum = 0
}

func New() rollinghash.Hash64 {
	return NewFromUint64Array(DefaultHash)
}

// NewFromUint32Array returns a buzhash based on the provided table uint32 values.
func NewFromUint64Array(b [256]uint64) rollinghash.Hash64 {
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
		d.sum = d.sum<<1 | d.sum>>63
		d.sum ^= d.bytehash[int(c)]
	}
	d.nRotate = uint(len(d.window)) % 64
	d.nRotateComplement = 64 - d.nRotate
	return len(d.window), nil
}

func (d *digest) Sum64() uint64 {
	return d.sum
}

func (d *digest) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
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

	d.sum = (d.sum<<1 | d.sum>>63) ^ (h0<<d.nRotate | h0>>d.nRotateComplement) ^ hn
}
