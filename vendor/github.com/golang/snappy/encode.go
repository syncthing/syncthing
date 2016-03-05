// Copyright 2011 The Snappy-Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snappy

import (
	"encoding/binary"
	"errors"
	"io"
)

// maxOffset limits how far copy back-references can go, the same as the C++
// code.
const maxOffset = 1 << 15

// emitLiteral writes a literal chunk and returns the number of bytes written.
func emitLiteral(dst, lit []byte) int {
	i, n := 0, uint(len(lit)-1)
	switch {
	case n < 60:
		dst[0] = uint8(n)<<2 | tagLiteral
		i = 1
	case n < 1<<8:
		dst[0] = 60<<2 | tagLiteral
		dst[1] = uint8(n)
		i = 2
	case n < 1<<16:
		dst[0] = 61<<2 | tagLiteral
		dst[1] = uint8(n)
		dst[2] = uint8(n >> 8)
		i = 3
	case n < 1<<24:
		dst[0] = 62<<2 | tagLiteral
		dst[1] = uint8(n)
		dst[2] = uint8(n >> 8)
		dst[3] = uint8(n >> 16)
		i = 4
	case int64(n) < 1<<32:
		dst[0] = 63<<2 | tagLiteral
		dst[1] = uint8(n)
		dst[2] = uint8(n >> 8)
		dst[3] = uint8(n >> 16)
		dst[4] = uint8(n >> 24)
		i = 5
	default:
		panic("snappy: source buffer is too long")
	}
	if copy(dst[i:], lit) != len(lit) {
		panic("snappy: destination buffer is too short")
	}
	return i + len(lit)
}

// emitCopy writes a copy chunk and returns the number of bytes written.
func emitCopy(dst []byte, offset, length int32) int {
	i := 0
	for length > 0 {
		x := length - 4
		if 0 <= x && x < 1<<3 && offset < 1<<11 {
			dst[i+0] = uint8(offset>>8)&0x07<<5 | uint8(x)<<2 | tagCopy1
			dst[i+1] = uint8(offset)
			i += 2
			break
		}

		x = length
		if x > 1<<6 {
			x = 1 << 6
		}
		dst[i+0] = uint8(x-1)<<2 | tagCopy2
		dst[i+1] = uint8(offset)
		dst[i+2] = uint8(offset >> 8)
		i += 3
		length -= x
	}
	return i
}

// Encode returns the encoded form of src. The returned slice may be a sub-
// slice of dst if dst was large enough to hold the entire encoded block.
// Otherwise, a newly allocated slice will be returned.
//
// It is valid to pass a nil dst.
func Encode(dst, src []byte) []byte {
	if n := MaxEncodedLen(len(src)); n < 0 {
		panic(ErrTooLarge)
	} else if len(dst) < n {
		dst = make([]byte, n)
	}

	// The block starts with the varint-encoded length of the decompressed bytes.
	d := binary.PutUvarint(dst, uint64(len(src)))

	for len(src) > 0 {
		p := src
		src = nil
		if len(p) > maxBlockSize {
			p, src = p[:maxBlockSize], p[maxBlockSize:]
		}
		d += encodeBlock(dst[d:], p)
	}
	return dst[:d]
}

// encodeBlock encodes a non-empty src to a guaranteed-large-enough dst. It
// assumes that the varint-encoded length of the decompressed bytes has already
// been written.
//
// It also assumes that:
//	len(dst) >= MaxEncodedLen(len(src)) &&
// 	0 < len(src) && len(src) <= maxBlockSize
func encodeBlock(dst, src []byte) (d int) {
	// Return early if src is short.
	if len(src) <= 4 {
		return emitLiteral(dst, src)
	}

	// Initialize the hash table. Its size ranges from 1<<8 to 1<<14 inclusive.
	const maxTableSize = 1 << 14
	shift, tableSize := uint(32-8), 1<<8
	for tableSize < maxTableSize && tableSize < len(src) {
		shift--
		tableSize *= 2
	}
	var table [maxTableSize]int32

	// Iterate over the source bytes.
	var (
		s   int32 // The iterator position.
		t   int32 // The last position with the same hash as s.
		lit int32 // The start position of any pending literal bytes.

		// Copied from the C++ snappy implementation:
		//
		// Heuristic match skipping: If 32 bytes are scanned with no matches
		// found, start looking only at every other byte. If 32 more bytes are
		// scanned, look at every third byte, etc.. When a match is found,
		// immediately go back to looking at every byte. This is a small loss
		// (~5% performance, ~0.1% density) for compressible data due to more
		// bookkeeping, but for non-compressible data (such as JPEG) it's a
		// huge win since the compressor quickly "realizes" the data is
		// incompressible and doesn't bother looking for matches everywhere.
		//
		// The "skip" variable keeps track of how many bytes there are since
		// the last match; dividing it by 32 (ie. right-shifting by five) gives
		// the number of bytes to move ahead for each iteration.
		skip uint32 = 32
	)
	for uint32(s+3) < uint32(len(src)) { // The uint32 conversions catch overflow from the +3.
		// Update the hash table.
		b0, b1, b2, b3 := src[s], src[s+1], src[s+2], src[s+3]
		h := uint32(b0) | uint32(b1)<<8 | uint32(b2)<<16 | uint32(b3)<<24
		p := &table[(h*0x1e35a7bd)>>shift]
		// We need to to store values in [-1, inf) in table. To save
		// some initialization time, (re)use the table's zero value
		// and shift the values against this zero: add 1 on writes,
		// subtract 1 on reads.
		t, *p = *p-1, s+1
		// If t is invalid or src[s:s+4] differs from src[t:t+4], accumulate a literal byte.
		if t < 0 || s-t >= maxOffset || b0 != src[t] || b1 != src[t+1] || b2 != src[t+2] || b3 != src[t+3] {
			s += int32(skip >> 5)
			skip++
			continue
		}
		skip = 32
		// Otherwise, we have a match. First, emit any pending literal bytes.
		if lit != s {
			d += emitLiteral(dst[d:], src[lit:s])
		}
		// Extend the match to be as long as possible.
		s0 := s
		s, t = s+4, t+4
		for int(s) < len(src) && src[s] == src[t] {
			s++
			t++
		}
		// Emit the copied bytes.
		d += emitCopy(dst[d:], s-t, s-s0)
		lit = s
	}

	// Emit any final pending literal bytes and return.
	if int(lit) != len(src) {
		d += emitLiteral(dst[d:], src[lit:])
	}
	return d
}

// MaxEncodedLen returns the maximum length of a snappy block, given its
// uncompressed length.
//
// It will return a negative value if srcLen is too large to encode.
func MaxEncodedLen(srcLen int) int {
	n := uint64(srcLen)
	if n > 0xffffffff {
		return -1
	}
	// Compressed data can be defined as:
	//    compressed := item* literal*
	//    item       := literal* copy
	//
	// The trailing literal sequence has a space blowup of at most 62/60
	// since a literal of length 60 needs one tag byte + one extra byte
	// for length information.
	//
	// Item blowup is trickier to measure. Suppose the "copy" op copies
	// 4 bytes of data. Because of a special check in the encoding code,
	// we produce a 4-byte copy only if the offset is < 65536. Therefore
	// the copy op takes 3 bytes to encode, and this type of item leads
	// to at most the 62/60 blowup for representing literals.
	//
	// Suppose the "copy" op copies 5 bytes of data. If the offset is big
	// enough, it will take 5 bytes to encode the copy op. Therefore the
	// worst case here is a one-byte literal followed by a five-byte copy.
	// That is, 6 bytes of input turn into 7 bytes of "compressed" data.
	//
	// This last factor dominates the blowup, so the final estimate is:
	n = 32 + n + n/6
	if n > 0xffffffff {
		return -1
	}
	return int(n)
}

var errClosed = errors.New("snappy: Writer is closed")

// NewWriter returns a new Writer that compresses to w.
//
// The Writer returned does not buffer writes. There is no need to Flush or
// Close such a Writer.
//
// Deprecated: the Writer returned is not suitable for many small writes, only
// for few large writes. Use NewBufferedWriter instead, which is efficient
// regardless of the frequency and shape of the writes, and remember to Close
// that Writer when done.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:    w,
		obuf: make([]byte, obufLen),
	}
}

// NewBufferedWriter returns a new Writer that compresses to w, using the
// framing format described at
// https://github.com/google/snappy/blob/master/framing_format.txt
//
// The Writer returned buffers writes. Users must call Close to guarantee all
// data has been forwarded to the underlying io.Writer. They may also call
// Flush zero or more times before calling Close.
func NewBufferedWriter(w io.Writer) *Writer {
	return &Writer{
		w:    w,
		ibuf: make([]byte, 0, maxBlockSize),
		obuf: make([]byte, obufLen),
	}
}

// Writer is an io.Writer than can write Snappy-compressed bytes.
type Writer struct {
	w   io.Writer
	err error

	// ibuf is a buffer for the incoming (uncompressed) bytes.
	//
	// Its use is optional. For backwards compatibility, Writers created by the
	// NewWriter function have ibuf == nil, do not buffer incoming bytes, and
	// therefore do not need to be Flush'ed or Close'd.
	ibuf []byte

	// obuf is a buffer for the outgoing (compressed) bytes.
	obuf []byte

	// wroteStreamHeader is whether we have written the stream header.
	wroteStreamHeader bool
}

// Reset discards the writer's state and switches the Snappy writer to write to
// w. This permits reusing a Writer rather than allocating a new one.
func (w *Writer) Reset(writer io.Writer) {
	w.w = writer
	w.err = nil
	if w.ibuf != nil {
		w.ibuf = w.ibuf[:0]
	}
	w.wroteStreamHeader = false
}

// Write satisfies the io.Writer interface.
func (w *Writer) Write(p []byte) (nRet int, errRet error) {
	if w.ibuf == nil {
		// Do not buffer incoming bytes. This does not perform or compress well
		// if the caller of Writer.Write writes many small slices. This
		// behavior is therefore deprecated, but still supported for backwards
		// compatibility with code that doesn't explicitly Flush or Close.
		return w.write(p)
	}

	// The remainder of this method is based on bufio.Writer.Write from the
	// standard library.

	for len(p) > (cap(w.ibuf)-len(w.ibuf)) && w.err == nil {
		var n int
		if len(w.ibuf) == 0 {
			// Large write, empty buffer.
			// Write directly from p to avoid copy.
			n, _ = w.write(p)
		} else {
			n = copy(w.ibuf[len(w.ibuf):cap(w.ibuf)], p)
			w.ibuf = w.ibuf[:len(w.ibuf)+n]
			w.Flush()
		}
		nRet += n
		p = p[n:]
	}
	if w.err != nil {
		return nRet, w.err
	}
	n := copy(w.ibuf[len(w.ibuf):cap(w.ibuf)], p)
	w.ibuf = w.ibuf[:len(w.ibuf)+n]
	nRet += n
	return nRet, nil
}

func (w *Writer) write(p []byte) (nRet int, errRet error) {
	if w.err != nil {
		return 0, w.err
	}
	for len(p) > 0 {
		obufStart := len(magicChunk)
		if !w.wroteStreamHeader {
			w.wroteStreamHeader = true
			copy(w.obuf, magicChunk)
			obufStart = 0
		}

		var uncompressed []byte
		if len(p) > maxBlockSize {
			uncompressed, p = p[:maxBlockSize], p[maxBlockSize:]
		} else {
			uncompressed, p = p, nil
		}
		checksum := crc(uncompressed)

		// Compress the buffer, discarding the result if the improvement
		// isn't at least 12.5%.
		compressed := Encode(w.obuf[obufHeaderLen:], uncompressed)
		chunkType := uint8(chunkTypeCompressedData)
		chunkLen := 4 + len(compressed)
		obufEnd := obufHeaderLen + len(compressed)
		if len(compressed) >= len(uncompressed)-len(uncompressed)/8 {
			chunkType = chunkTypeUncompressedData
			chunkLen = 4 + len(uncompressed)
			obufEnd = obufHeaderLen
		}

		// Fill in the per-chunk header that comes before the body.
		w.obuf[len(magicChunk)+0] = chunkType
		w.obuf[len(magicChunk)+1] = uint8(chunkLen >> 0)
		w.obuf[len(magicChunk)+2] = uint8(chunkLen >> 8)
		w.obuf[len(magicChunk)+3] = uint8(chunkLen >> 16)
		w.obuf[len(magicChunk)+4] = uint8(checksum >> 0)
		w.obuf[len(magicChunk)+5] = uint8(checksum >> 8)
		w.obuf[len(magicChunk)+6] = uint8(checksum >> 16)
		w.obuf[len(magicChunk)+7] = uint8(checksum >> 24)

		if _, err := w.w.Write(w.obuf[obufStart:obufEnd]); err != nil {
			w.err = err
			return nRet, err
		}
		if chunkType == chunkTypeUncompressedData {
			if _, err := w.w.Write(uncompressed); err != nil {
				w.err = err
				return nRet, err
			}
		}
		nRet += len(uncompressed)
	}
	return nRet, nil
}

// Flush flushes the Writer to its underlying io.Writer.
func (w *Writer) Flush() error {
	if w.err != nil {
		return w.err
	}
	if len(w.ibuf) == 0 {
		return nil
	}
	w.write(w.ibuf)
	w.ibuf = w.ibuf[:0]
	return w.err
}

// Close calls Flush and then closes the Writer.
func (w *Writer) Close() error {
	w.Flush()
	ret := w.err
	if w.err == nil {
		w.err = errClosed
	}
	return ret
}
