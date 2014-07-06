// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"encoding/binary"
	"fmt"
)

type vType int

func (t vType) String() string {
	switch t {
	case tDel:
		return "d"
	case tVal:
		return "v"
	}
	return "x"
}

// Value types encoded as the last component of internal keys.
// Don't modify; this value are saved to disk.
const (
	tDel vType = iota
	tVal
)

// tSeek defines the vType that should be passed when constructing an
// internal key for seeking to a particular sequence number (since we
// sort sequence numbers in decreasing order and the value type is
// embedded as the low 8 bits in the sequence number in internal keys,
// we need to use the highest-numbered ValueType, not the lowest).
const tSeek = tVal

const (
	// Maximum value possible for sequence number; the 8-bits are
	// used by value type, so its can packed together in single
	// 64-bit integer.
	kMaxSeq uint64 = (uint64(1) << 56) - 1
	// Maximum value possible for packed sequence number and type.
	kMaxNum uint64 = (kMaxSeq << 8) | uint64(tSeek)
)

// Maximum number encoded in bytes.
var kMaxNumBytes = make([]byte, 8)

func init() {
	binary.LittleEndian.PutUint64(kMaxNumBytes, kMaxNum)
}

type iKey []byte

func newIKey(ukey []byte, seq uint64, t vType) iKey {
	if seq > kMaxSeq || t > tVal {
		panic("invalid seq number or value type")
	}

	b := make(iKey, len(ukey)+8)
	copy(b, ukey)
	binary.LittleEndian.PutUint64(b[len(ukey):], (seq<<8)|uint64(t))
	return b
}

func parseIkey(p []byte) (ukey []byte, seq uint64, t vType, ok bool) {
	if len(p) < 8 {
		return
	}
	num := binary.LittleEndian.Uint64(p[len(p)-8:])
	seq, t = uint64(num>>8), vType(num&0xff)
	if t > tVal {
		return
	}
	ukey = p[:len(p)-8]
	ok = true
	return
}

func validIkey(p []byte) bool {
	_, _, _, ok := parseIkey(p)
	return ok
}

func (p iKey) assert() {
	if p == nil {
		panic("nil iKey")
	}
	if len(p) < 8 {
		panic(fmt.Sprintf("invalid iKey %q, len=%d", []byte(p), len(p)))
	}
}

func (p iKey) ok() bool {
	if len(p) < 8 {
		return false
	}
	_, _, ok := p.parseNum()
	return ok
}

func (p iKey) ukey() []byte {
	p.assert()
	return p[:len(p)-8]
}

func (p iKey) num() uint64 {
	p.assert()
	return binary.LittleEndian.Uint64(p[len(p)-8:])
}

func (p iKey) parseNum() (seq uint64, t vType, ok bool) {
	if p == nil {
		panic("nil iKey")
	}
	if len(p) < 8 {
		return
	}
	num := p.num()
	seq, t = uint64(num>>8), vType(num&0xff)
	if t > tVal {
		return 0, 0, false
	}
	ok = true
	return
}

func (p iKey) String() string {
	if len(p) == 0 {
		return "<nil>"
	}
	if seq, t, ok := p.parseNum(); ok {
		return fmt.Sprintf("%s,%s%d", shorten(string(p.ukey())), t, seq)
	}
	return "<invalid>"
}
