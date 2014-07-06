// Copyright 2011 The Snappy-Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snappy

import (
	"encoding/binary"
	"errors"
)

// ErrCorrupt reports that the input is invalid.
var ErrCorrupt = errors.New("snappy: corrupt input")

// DecodedLen returns the length of the decoded block.
func DecodedLen(src []byte) (int, error) {
	v, _, err := decodedLen(src)
	return v, err
}

// decodedLen returns the length of the decoded block and the number of bytes
// that the length header occupied.
func decodedLen(src []byte) (blockLen, headerLen int, err error) {
	v, n := binary.Uvarint(src)
	if n == 0 {
		return 0, 0, ErrCorrupt
	}
	if uint64(int(v)) != v {
		return 0, 0, errors.New("snappy: decoded block is too large")
	}
	return int(v), n, nil
}

// Decode returns the decoded form of src. The returned slice may be a sub-
// slice of dst if dst was large enough to hold the entire decoded block.
// Otherwise, a newly allocated slice will be returned.
// It is valid to pass a nil dst.
func Decode(dst, src []byte) ([]byte, error) {
	dLen, s, err := decodedLen(src)
	if err != nil {
		return nil, err
	}
	if len(dst) < dLen {
		dst = make([]byte, dLen)
	}

	var d, offset, length int
	for s < len(src) {
		switch src[s] & 0x03 {
		case tagLiteral:
			x := uint(src[s] >> 2)
			switch {
			case x < 60:
				s += 1
			case x == 60:
				s += 2
				if s > len(src) {
					return nil, ErrCorrupt
				}
				x = uint(src[s-1])
			case x == 61:
				s += 3
				if s > len(src) {
					return nil, ErrCorrupt
				}
				x = uint(src[s-2]) | uint(src[s-1])<<8
			case x == 62:
				s += 4
				if s > len(src) {
					return nil, ErrCorrupt
				}
				x = uint(src[s-3]) | uint(src[s-2])<<8 | uint(src[s-1])<<16
			case x == 63:
				s += 5
				if s > len(src) {
					return nil, ErrCorrupt
				}
				x = uint(src[s-4]) | uint(src[s-3])<<8 | uint(src[s-2])<<16 | uint(src[s-1])<<24
			}
			length = int(x + 1)
			if length <= 0 {
				return nil, errors.New("snappy: unsupported literal length")
			}
			if length > len(dst)-d || length > len(src)-s {
				return nil, ErrCorrupt
			}
			copy(dst[d:], src[s:s+length])
			d += length
			s += length
			continue

		case tagCopy1:
			s += 2
			if s > len(src) {
				return nil, ErrCorrupt
			}
			length = 4 + int(src[s-2])>>2&0x7
			offset = int(src[s-2])&0xe0<<3 | int(src[s-1])

		case tagCopy2:
			s += 3
			if s > len(src) {
				return nil, ErrCorrupt
			}
			length = 1 + int(src[s-3])>>2
			offset = int(src[s-2]) | int(src[s-1])<<8

		case tagCopy4:
			return nil, errors.New("snappy: unsupported COPY_4 tag")
		}

		end := d + length
		if offset > d || end > len(dst) {
			return nil, ErrCorrupt
		}
		for ; d < end; d++ {
			dst[d] = dst[d-offset]
		}
	}
	if d != dLen {
		return nil, ErrCorrupt
	}
	return dst[:d], nil
}
