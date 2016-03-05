// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"encoding/binary"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

// ErrInternalKeyCorrupted records internal key corruption.
type ErrInternalKeyCorrupted struct {
	Ikey   []byte
	Reason string
}

func (e *ErrInternalKeyCorrupted) Error() string {
	return fmt.Sprintf("leveldb: internal key %q corrupted: %s", e.Ikey, e.Reason)
}

func newErrInternalKeyCorrupted(ikey []byte, reason string) error {
	return errors.NewErrCorrupted(storage.FileDesc{}, &ErrInternalKeyCorrupted{append([]byte{}, ikey...), reason})
}

type keyType uint

func (kt keyType) String() string {
	switch kt {
	case keyTypeDel:
		return "d"
	case keyTypeVal:
		return "v"
	}
	return "x"
}

// Value types encoded as the last component of internal keys.
// Don't modify; this value are saved to disk.
const (
	keyTypeDel keyType = iota
	keyTypeVal
)

// keyTypeSeek defines the keyType that should be passed when constructing an
// internal key for seeking to a particular sequence number (since we
// sort sequence numbers in decreasing order and the value type is
// embedded as the low 8 bits in the sequence number in internal keys,
// we need to use the highest-numbered ValueType, not the lowest).
const keyTypeSeek = keyTypeVal

const (
	// Maximum value possible for sequence number; the 8-bits are
	// used by value type, so its can packed together in single
	// 64-bit integer.
	keyMaxSeq = (uint64(1) << 56) - 1
	// Maximum value possible for packed sequence number and type.
	keyMaxNum = (keyMaxSeq << 8) | uint64(keyTypeSeek)
)

// Maximum number encoded in bytes.
var keyMaxNumBytes = make([]byte, 8)

func init() {
	binary.LittleEndian.PutUint64(keyMaxNumBytes, keyMaxNum)
}

type internalKey []byte

func makeInternalKey(dst, ukey []byte, seq uint64, kt keyType) internalKey {
	if seq > keyMaxSeq {
		panic("leveldb: invalid sequence number")
	} else if kt > keyTypeVal {
		panic("leveldb: invalid type")
	}

	if n := len(ukey) + 8; cap(dst) < n {
		dst = make([]byte, n)
	} else {
		dst = dst[:n]
	}
	copy(dst, ukey)
	binary.LittleEndian.PutUint64(dst[len(ukey):], (seq<<8)|uint64(kt))
	return internalKey(dst)
}

func parseInternalKey(ik []byte) (ukey []byte, seq uint64, kt keyType, err error) {
	if len(ik) < 8 {
		return nil, 0, 0, newErrInternalKeyCorrupted(ik, "invalid length")
	}
	num := binary.LittleEndian.Uint64(ik[len(ik)-8:])
	seq, kt = uint64(num>>8), keyType(num&0xff)
	if kt > keyTypeVal {
		return nil, 0, 0, newErrInternalKeyCorrupted(ik, "invalid type")
	}
	ukey = ik[:len(ik)-8]
	return
}

func validInternalKey(ik []byte) bool {
	_, _, _, err := parseInternalKey(ik)
	return err == nil
}

func (ik internalKey) assert() {
	if ik == nil {
		panic("leveldb: nil internalKey")
	}
	if len(ik) < 8 {
		panic(fmt.Sprintf("leveldb: internal key %q, len=%d: invalid length", []byte(ik), len(ik)))
	}
}

func (ik internalKey) ukey() []byte {
	ik.assert()
	return ik[:len(ik)-8]
}

func (ik internalKey) num() uint64 {
	ik.assert()
	return binary.LittleEndian.Uint64(ik[len(ik)-8:])
}

func (ik internalKey) parseNum() (seq uint64, kt keyType) {
	num := ik.num()
	seq, kt = uint64(num>>8), keyType(num&0xff)
	if kt > keyTypeVal {
		panic(fmt.Sprintf("leveldb: internal key %q, len=%d: invalid type %#x", []byte(ik), len(ik), kt))
	}
	return
}

func (ik internalKey) String() string {
	if ik == nil {
		return "<nil>"
	}

	if ukey, seq, kt, err := parseInternalKey(ik); err == nil {
		return fmt.Sprintf("%s,%s%d", shorten(string(ukey)), kt, seq)
	}
	return "<invalid>"
}
