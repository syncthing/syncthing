// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb/memdb"
)

var (
	errBatchTooShort  = errors.New("leveldb: batch is too short")
	errBatchBadRecord = errors.New("leveldb: bad record in batch")
)

const kBatchHdrLen = 8 + 4

type batchReplay interface {
	put(key, value []byte, seq uint64)
	delete(key []byte, seq uint64)
}

// Batch is a write batch.
type Batch struct {
	buf        []byte
	rLen, bLen int
	seq        uint64
	sync       bool
}

func (b *Batch) grow(n int) {
	off := len(b.buf)
	if off == 0 {
		// include headers
		off = kBatchHdrLen
		n += off
	}
	if cap(b.buf)-off >= n {
		return
	}
	buf := make([]byte, 2*cap(b.buf)+n)
	copy(buf, b.buf)
	b.buf = buf[:off]
}

func (b *Batch) appendRec(t vType, key, value []byte) {
	n := 1 + binary.MaxVarintLen32 + len(key)
	if t == tVal {
		n += binary.MaxVarintLen32 + len(value)
	}
	b.grow(n)
	off := len(b.buf)
	buf := b.buf[:off+n]
	buf[off] = byte(t)
	off += 1
	off += binary.PutUvarint(buf[off:], uint64(len(key)))
	copy(buf[off:], key)
	off += len(key)
	if t == tVal {
		off += binary.PutUvarint(buf[off:], uint64(len(value)))
		copy(buf[off:], value)
		off += len(value)
	}
	b.buf = buf[:off]
	b.rLen++
	//  Include 8-byte ikey header
	b.bLen += len(key) + len(value) + 8
}

// Put appends 'put operation' of the given key/value pair to the batch.
// It is safe to modify the contents of the argument after Put returns.
func (b *Batch) Put(key, value []byte) {
	b.appendRec(tVal, key, value)
}

// Delete appends 'delete operation' of the given key to the batch.
// It is safe to modify the contents of the argument after Delete returns.
func (b *Batch) Delete(key []byte) {
	b.appendRec(tDel, key, nil)
}

// Reset resets the batch.
func (b *Batch) Reset() {
	b.buf = nil
	b.seq = 0
	b.rLen = 0
	b.bLen = 0
	b.sync = false
}

func (b *Batch) init(sync bool) {
	b.sync = sync
}

func (b *Batch) put(key, value []byte, seq uint64) {
	if b.rLen == 0 {
		b.seq = seq
	}
	b.Put(key, value)
}

func (b *Batch) delete(key []byte, seq uint64) {
	if b.rLen == 0 {
		b.seq = seq
	}
	b.Delete(key)
}

func (b *Batch) append(p *Batch) {
	if p.rLen > 0 {
		b.grow(len(p.buf) - kBatchHdrLen)
		b.buf = append(b.buf, p.buf[kBatchHdrLen:]...)
		b.rLen += p.rLen
	}
	if p.sync {
		b.sync = true
	}
}

func (b *Batch) len() int {
	return b.rLen
}

func (b *Batch) size() int {
	return b.bLen
}

func (b *Batch) encode() []byte {
	b.grow(0)
	binary.LittleEndian.PutUint64(b.buf, b.seq)
	binary.LittleEndian.PutUint32(b.buf[8:], uint32(b.rLen))

	return b.buf
}

func (b *Batch) decode(buf []byte) error {
	if len(buf) < kBatchHdrLen {
		return errBatchTooShort
	}

	b.seq = binary.LittleEndian.Uint64(buf)
	b.rLen = int(binary.LittleEndian.Uint32(buf[8:]))
	// No need to be precise at this point, it won't be used anyway
	b.bLen = len(buf) - kBatchHdrLen
	b.buf = buf

	return nil
}

func (b *Batch) decodeRec(f func(i int, t vType, key, value []byte)) error {
	off := kBatchHdrLen
	for i := 0; i < b.rLen; i++ {
		if off >= len(b.buf) {
			return errors.New("leveldb: invalid batch record length")
		}

		t := vType(b.buf[off])
		if t > tVal {
			return errors.New("leveldb: invalid batch record type in batch")
		}
		off += 1

		x, n := binary.Uvarint(b.buf[off:])
		off += n
		if n <= 0 || off+int(x) > len(b.buf) {
			return errBatchBadRecord
		}
		key := b.buf[off : off+int(x)]
		off += int(x)

		var value []byte
		if t == tVal {
			x, n := binary.Uvarint(b.buf[off:])
			off += n
			if n <= 0 || off+int(x) > len(b.buf) {
				return errBatchBadRecord
			}
			value = b.buf[off : off+int(x)]
			off += int(x)
		}

		f(i, t, key, value)
	}

	return nil
}

func (b *Batch) replay(to batchReplay) error {
	return b.decodeRec(func(i int, t vType, key, value []byte) {
		switch t {
		case tVal:
			to.put(key, value, b.seq+uint64(i))
		case tDel:
			to.delete(key, b.seq+uint64(i))
		}
	})
}

func (b *Batch) memReplay(to *memdb.DB) error {
	return b.decodeRec(func(i int, t vType, key, value []byte) {
		ikey := newIKey(key, b.seq+uint64(i), t)
		to.Put(ikey, value)
	})
}

func (b *Batch) revertMemReplay(to *memdb.DB) error {
	return b.decodeRec(func(i int, t vType, key, value []byte) {
		ikey := newIKey(key, b.seq+uint64(i), t)
		to.Delete(ikey)
	})
}
