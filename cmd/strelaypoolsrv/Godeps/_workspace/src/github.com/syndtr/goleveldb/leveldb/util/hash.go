// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"bytes"
	"encoding/binary"
)

// Hash return hash of the given data.
func Hash(data []byte, seed uint32) uint32 {
	// Similar to murmur hash
	var m uint32 = 0xc6a4a793
	var r uint32 = 24
	h := seed ^ (uint32(len(data)) * m)

	buf := bytes.NewBuffer(data)
	for buf.Len() >= 4 {
		var w uint32
		binary.Read(buf, binary.LittleEndian, &w)
		h += w
		h *= m
		h ^= (h >> 16)
	}

	rest := buf.Bytes()
	switch len(rest) {
	default:
		panic("not reached")
	case 3:
		h += uint32(rest[2]) << 16
		fallthrough
	case 2:
		h += uint32(rest[1]) << 8
		fallthrough
	case 1:
		h += uint32(rest[0])
		h *= m
		h ^= (h >> r)
	case 0:
	}

	return h
}
