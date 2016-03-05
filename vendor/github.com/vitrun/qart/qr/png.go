// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qr

// PNG writer for QR codes.

import (
	"bytes"
	"encoding/binary"
	"hash"
	"hash/crc32"
)

// PNG returns a PNG image displaying the code.
//
// PNG uses a custom encoder tailored to QR codes.
// Its compressed size is about 2x away from optimal,
// but it runs about 20x faster than calling png.Encode
// on c.Image().
func (c *Code) PNG() []byte {
	var p pngWriter
	return p.encode(c)
}

type pngWriter struct {
	tmp   [16]byte
	wctmp [4]byte
	buf   bytes.Buffer
	zlib  bitWriter
	crc   hash.Hash32
}

var pngHeader = []byte("\x89PNG\r\n\x1a\n")

func (w *pngWriter) encode(c *Code) []byte {
	scale := c.Scale
	siz := c.Size

	w.buf.Reset()

	// Header
	w.buf.Write(pngHeader)

	// Header block
	binary.BigEndian.PutUint32(w.tmp[0:4], uint32((siz+8)*scale))
	binary.BigEndian.PutUint32(w.tmp[4:8], uint32((siz+8)*scale))
	w.tmp[8] = 1 // 1-bit
	w.tmp[9] = 0 // gray
	w.tmp[10] = 0
	w.tmp[11] = 0
	w.tmp[12] = 0
	w.writeChunk("IHDR", w.tmp[:13])

	// Comment
	w.writeChunk("tEXt", comment)

	// Data
	w.zlib.writeCode(c)
	w.writeChunk("IDAT", w.zlib.bytes.Bytes())

	// End
	w.writeChunk("IEND", nil)

	return w.buf.Bytes()
}

var comment = []byte("Software\x00QR-PNG http://qr.swtch.com/")

func (w *pngWriter) writeChunk(name string, data []byte) {
	if w.crc == nil {
		w.crc = crc32.NewIEEE()
	}
	binary.BigEndian.PutUint32(w.wctmp[0:4], uint32(len(data)))
	w.buf.Write(w.wctmp[0:4])
	w.crc.Reset()
	copy(w.wctmp[0:4], name)
	w.buf.Write(w.wctmp[0:4])
	w.crc.Write(w.wctmp[0:4])
	w.buf.Write(data)
	w.crc.Write(data)
	crc := w.crc.Sum32()
	binary.BigEndian.PutUint32(w.wctmp[0:4], crc)
	w.buf.Write(w.wctmp[0:4])
}

func (b *bitWriter) writeCode(c *Code) {
	const ftNone = 0

	b.adler32.Reset()
	b.bytes.Reset()
	b.nbit = 0

	scale := c.Scale
	siz := c.Size

	// zlib header
	b.tmp[0] = 0x78
	b.tmp[1] = 0
	b.tmp[1] += uint8(31 - (uint16(b.tmp[0])<<8+uint16(b.tmp[1]))%31)
	b.bytes.Write(b.tmp[0:2])

	// Start flate block.
	b.writeBits(1, 1, false) // final block
	b.writeBits(1, 2, false) // compressed, fixed Huffman tables

	// White border.
	// First row.
	b.byte(ftNone)
	n := (scale*(siz+8) + 7) / 8
	b.byte(255)
	b.repeat(n-1, 1)
	// 4*scale rows total.
	b.repeat((4*scale-1)*(1+n), 1+n)

	for i := 0; i < 4*scale; i++ {
		b.adler32.WriteNByte(ftNone, 1)
		b.adler32.WriteNByte(255, n)
	}

	row := make([]byte, 1+n)
	for y := 0; y < siz; y++ {
		row[0] = ftNone
		j := 1
		var z uint8
		nz := 0
		for x := -4; x < siz+4; x++ {
			// Raw data.
			for i := 0; i < scale; i++ {
				z <<= 1
				if !c.Black(x, y) {
					z |= 1
				}
				if nz++; nz == 8 {
					row[j] = z
					j++
					nz = 0
				}
			}
		}
		if j < len(row) {
			row[j] = z
		}
		for _, z := range row {
			b.byte(z)
		}

		// Scale-1 copies.
		b.repeat((scale-1)*(1+n), 1+n)

		b.adler32.WriteN(row, scale)
	}

	// White border.
	// First row.
	b.byte(ftNone)
	b.byte(255)
	b.repeat(n-1, 1)
	// 4*scale rows total.
	b.repeat((4*scale-1)*(1+n), 1+n)

	for i := 0; i < 4*scale; i++ {
		b.adler32.WriteNByte(ftNone, 1)
		b.adler32.WriteNByte(255, n)
	}

	// End of block.
	b.hcode(256)
	b.flushBits()

	// adler32
	binary.BigEndian.PutUint32(b.tmp[0:], b.adler32.Sum32())
	b.bytes.Write(b.tmp[0:4])
}

// A bitWriter is a write buffer for bit-oriented data like deflate.
type bitWriter struct {
	bytes bytes.Buffer
	bit   uint32
	nbit  uint

	tmp     [4]byte
	adler32 adigest
}

func (b *bitWriter) writeBits(bit uint32, nbit uint, rev bool) {
	// reverse, for huffman codes
	if rev {
		br := uint32(0)
		for i := uint(0); i < nbit; i++ {
			br |= ((bit >> i) & 1) << (nbit - 1 - i)
		}
		bit = br
	}
	b.bit |= bit << b.nbit
	b.nbit += nbit
	for b.nbit >= 8 {
		b.bytes.WriteByte(byte(b.bit))
		b.bit >>= 8
		b.nbit -= 8
	}
}

func (b *bitWriter) flushBits() {
	if b.nbit > 0 {
		b.bytes.WriteByte(byte(b.bit))
		b.nbit = 0
		b.bit = 0
	}
}

func (b *bitWriter) hcode(v int) {
	/*
	   Lit Value    Bits        Codes
	   ---------    ----        -----
	     0 - 143     8          00110000 through
	                            10111111
	   144 - 255     9          110010000 through
	                            111111111
	   256 - 279     7          0000000 through
	                            0010111
	   280 - 287     8          11000000 through
	                            11000111
	*/
	switch {
	case v <= 143:
		b.writeBits(uint32(v)+0x30, 8, true)
	case v <= 255:
		b.writeBits(uint32(v-144)+0x190, 9, true)
	case v <= 279:
		b.writeBits(uint32(v-256)+0, 7, true)
	case v <= 287:
		b.writeBits(uint32(v-280)+0xc0, 8, true)
	default:
		panic("invalid hcode")
	}
}

func (b *bitWriter) byte(x byte) {
	b.hcode(int(x))
}

func (b *bitWriter) codex(c int, val int, nx uint) {
	b.hcode(c + val>>nx)
	b.writeBits(uint32(val)&(1<<nx-1), nx, false)
}

func (b *bitWriter) repeat(n, d int) {
	for ; n >= 258+3; n -= 258 {
		b.repeat1(258, d)
	}
	if n > 258 {
		// 258 < n < 258+3
		b.repeat1(10, d)
		b.repeat1(n-10, d)
		return
	}
	if n < 3 {
		panic("invalid flate repeat")
	}
	b.repeat1(n, d)
}

func (b *bitWriter) repeat1(n, d int) {
	/*
	        Extra               Extra               Extra
	   Code Bits Length(s) Code Bits Lengths   Code Bits Length(s)
	   ---- ---- ------     ---- ---- -------   ---- ---- -------
	    257   0     3       267   1   15,16     277   4   67-82
	    258   0     4       268   1   17,18     278   4   83-98
	    259   0     5       269   2   19-22     279   4   99-114
	    260   0     6       270   2   23-26     280   4  115-130
	    261   0     7       271   2   27-30     281   5  131-162
	    262   0     8       272   2   31-34     282   5  163-194
	    263   0     9       273   3   35-42     283   5  195-226
	    264   0    10       274   3   43-50     284   5  227-257
	    265   1  11,12      275   3   51-58     285   0    258
	    266   1  13,14      276   3   59-66
	*/
	switch {
	case n <= 10:
		b.codex(257, n-3, 0)
	case n <= 18:
		b.codex(265, n-11, 1)
	case n <= 34:
		b.codex(269, n-19, 2)
	case n <= 66:
		b.codex(273, n-35, 3)
	case n <= 130:
		b.codex(277, n-67, 4)
	case n <= 257:
		b.codex(281, n-131, 5)
	case n == 258:
		b.hcode(285)
	default:
		panic("invalid repeat length")
	}

	/*
	        Extra           Extra               Extra
	   Code Bits Dist  Code Bits   Dist     Code Bits Distance
	   ---- ---- ----  ---- ----  ------    ---- ---- --------
	     0   0    1     10   4     33-48    20    9   1025-1536
	     1   0    2     11   4     49-64    21    9   1537-2048
	     2   0    3     12   5     65-96    22   10   2049-3072
	     3   0    4     13   5     97-128   23   10   3073-4096
	     4   1   5,6    14   6    129-192   24   11   4097-6144
	     5   1   7,8    15   6    193-256   25   11   6145-8192
	     6   2   9-12   16   7    257-384   26   12  8193-12288
	     7   2  13-16   17   7    385-512   27   12 12289-16384
	     8   3  17-24   18   8    513-768   28   13 16385-24576
	     9   3  25-32   19   8   769-1024   29   13 24577-32768
	*/
	if d <= 4 {
		b.writeBits(uint32(d-1), 5, true)
	} else if d <= 32768 {
		nbit := uint(16)
		for d <= 1<<(nbit-1) {
			nbit--
		}
		v := uint32(d - 1)
		v &^= 1 << (nbit - 1)      // top bit is implicit
		code := uint32(2*nbit - 2) // second bit is low bit of code
		code |= v >> (nbit - 2)
		v &^= 1 << (nbit - 2)
		b.writeBits(code, 5, true)
		// rest of bits follow
		b.writeBits(uint32(v), nbit-2, false)
	} else {
		panic("invalid repeat distance")
	}
}

func (b *bitWriter) run(v byte, n int) {
	if n == 0 {
		return
	}
	b.byte(v)
	if n-1 < 3 {
		for i := 0; i < n-1; i++ {
			b.byte(v)
		}
	} else {
		b.repeat(n-1, 1)
	}
}

type adigest struct {
	a, b uint32
}

func (d *adigest) Reset() { d.a, d.b = 1, 0 }

const amod = 65521

func aupdate(a, b uint32, pi byte, n int) (aa, bb uint32) {
	// TODO(rsc): 6g doesn't do magic multiplies for b %= amod,
	// only for b = b%amod.

	// invariant: a, b < amod
	if pi == 0 {
		b += uint32(n%amod) * a
		b = b % amod
		return a, b
	}

	// n times:
	//	a += pi
	//	b += a
	// is same as
	//	b += n*a + n*(n+1)/2*pi
	//	a += n*pi
	m := uint32(n)
	b += (m % amod) * a
	b = b % amod
	b += (m * (m + 1) / 2) % amod * uint32(pi)
	b = b % amod
	a += (m % amod) * uint32(pi)
	a = a % amod
	return a, b
}

func afinish(a, b uint32) uint32 {
	return b<<16 | a
}

func (d *adigest) WriteN(p []byte, n int) {
	for i := 0; i < n; i++ {
		for _, pi := range p {
			d.a, d.b = aupdate(d.a, d.b, pi, 1)
		}
	}
}

func (d *adigest) WriteNByte(pi byte, n int) {
	d.a, d.b = aupdate(d.a, d.b, pi, n)
}

func (d *adigest) Sum32() uint32 { return afinish(d.a, d.b) }
