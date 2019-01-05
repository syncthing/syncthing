/*
 * Minio Cloud Storage, (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sha256

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"runtime"
)

// Size - The size of a SHA256 checksum in bytes.
const Size = 32

// BlockSize - The blocksize of SHA256 in bytes.
const BlockSize = 64

const (
	chunk = BlockSize
	init0 = 0x6A09E667
	init1 = 0xBB67AE85
	init2 = 0x3C6EF372
	init3 = 0xA54FF53A
	init4 = 0x510E527F
	init5 = 0x9B05688C
	init6 = 0x1F83D9AB
	init7 = 0x5BE0CD19
)

// digest represents the partial evaluation of a checksum.
type digest struct {
	h   [8]uint32
	x   [chunk]byte
	nx  int
	len uint64
}

// Reset digest back to default
func (d *digest) Reset() {
	d.h[0] = init0
	d.h[1] = init1
	d.h[2] = init2
	d.h[3] = init3
	d.h[4] = init4
	d.h[5] = init5
	d.h[6] = init6
	d.h[7] = init7
	d.nx = 0
	d.len = 0
}

type blockfuncType int

const (
	blockfuncGeneric blockfuncType = iota
	blockfuncAvx512  blockfuncType = iota
	blockfuncAvx2    blockfuncType = iota
	blockfuncAvx     blockfuncType = iota
	blockfuncSsse    blockfuncType = iota
	blockfuncSha     blockfuncType = iota
	blockfuncArm     blockfuncType = iota
)

var blockfunc blockfuncType

func block(dig *digest, p []byte) {
	if blockfunc == blockfuncSha {
		blockShaGo(dig, p)
	} else if blockfunc == blockfuncAvx2 {
		blockAvx2Go(dig, p)
	} else if blockfunc == blockfuncAvx {
		blockAvxGo(dig, p)
	} else if blockfunc == blockfuncSsse {
		blockSsseGo(dig, p)
	} else if blockfunc == blockfuncArm {
		blockArmGo(dig, p)
	} else if blockfunc == blockfuncGeneric {
		blockGeneric(dig, p)
	}
}

func init() {
	is386bit := runtime.GOARCH == "386"
	isARM := runtime.GOARCH == "arm"
	switch {
	case is386bit || isARM:
		blockfunc = blockfuncGeneric
	case sha && ssse3 && sse41:
		blockfunc = blockfuncSha
	case avx2:
		blockfunc = blockfuncAvx2
	case avx:
		blockfunc = blockfuncAvx
	case ssse3:
		blockfunc = blockfuncSsse
	case armSha:
		blockfunc = blockfuncArm
	default:
		blockfunc = blockfuncGeneric
	}
}

// New returns a new hash.Hash computing the SHA256 checksum.
func New() hash.Hash {
	if blockfunc != blockfuncGeneric {
		d := new(digest)
		d.Reset()
		return d
	}
	// Fallback to the standard golang implementation
	// if no features were found.
	return sha256.New()
}

// Sum256 - single caller sha256 helper
func Sum256(data []byte) (result [Size]byte) {
	var d digest
	d.Reset()
	d.Write(data)
	result = d.checkSum()
	return
}

// Return size of checksum
func (d *digest) Size() int { return Size }

// Return blocksize of checksum
func (d *digest) BlockSize() int { return BlockSize }

// Write to digest
func (d *digest) Write(p []byte) (nn int, err error) {
	nn = len(p)
	d.len += uint64(nn)
	if d.nx > 0 {
		n := copy(d.x[d.nx:], p)
		d.nx += n
		if d.nx == chunk {
			block(d, d.x[:])
			d.nx = 0
		}
		p = p[n:]
	}
	if len(p) >= chunk {
		n := len(p) &^ (chunk - 1)
		block(d, p[:n])
		p = p[n:]
	}
	if len(p) > 0 {
		d.nx = copy(d.x[:], p)
	}
	return
}

// Return sha256 sum in bytes
func (d *digest) Sum(in []byte) []byte {
	// Make a copy of d0 so that caller can keep writing and summing.
	d0 := *d
	hash := d0.checkSum()
	return append(in, hash[:]...)
}

// Intermediate checksum function
func (d *digest) checkSum() (digest [Size]byte) {
	n := d.nx

	var k [64]byte
	copy(k[:], d.x[:n])

	k[n] = 0x80

	if n >= 56 {
		block(d, k[:])

		// clear block buffer - go compiles this to optimal 1x xorps + 4x movups
		// unfortunately expressing this more succinctly results in much worse code
		k[0] = 0
		k[1] = 0
		k[2] = 0
		k[3] = 0
		k[4] = 0
		k[5] = 0
		k[6] = 0
		k[7] = 0
		k[8] = 0
		k[9] = 0
		k[10] = 0
		k[11] = 0
		k[12] = 0
		k[13] = 0
		k[14] = 0
		k[15] = 0
		k[16] = 0
		k[17] = 0
		k[18] = 0
		k[19] = 0
		k[20] = 0
		k[21] = 0
		k[22] = 0
		k[23] = 0
		k[24] = 0
		k[25] = 0
		k[26] = 0
		k[27] = 0
		k[28] = 0
		k[29] = 0
		k[30] = 0
		k[31] = 0
		k[32] = 0
		k[33] = 0
		k[34] = 0
		k[35] = 0
		k[36] = 0
		k[37] = 0
		k[38] = 0
		k[39] = 0
		k[40] = 0
		k[41] = 0
		k[42] = 0
		k[43] = 0
		k[44] = 0
		k[45] = 0
		k[46] = 0
		k[47] = 0
		k[48] = 0
		k[49] = 0
		k[50] = 0
		k[51] = 0
		k[52] = 0
		k[53] = 0
		k[54] = 0
		k[55] = 0
		k[56] = 0
		k[57] = 0
		k[58] = 0
		k[59] = 0
		k[60] = 0
		k[61] = 0
		k[62] = 0
		k[63] = 0
	}
	binary.BigEndian.PutUint64(k[56:64], uint64(d.len)<<3)
	block(d, k[:])

	{
		const i = 0
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 1
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 2
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 3
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 4
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 5
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 6
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}
	{
		const i = 7
		binary.BigEndian.PutUint32(digest[i*4:i*4+4], d.h[i])
	}

	return
}
