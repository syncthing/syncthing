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
	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

// ErrBatchCorrupted records reason of batch corruption.
type ErrBatchCorrupted struct {
	Reason string
}

func (e *ErrBatchCorrupted) Error() string {
	return fmt.Sprintf("leveldb: batch corrupted: %s", e.Reason)
}

func newErrBatchCorrupted(reason string) error {
	return errors.NewErrCorrupted(storage.FileDesc{}, &ErrBatchCorrupted{reason})
}

const (
	batchHdrLen  = 8 + 4
	batchGrowRec = 3000
)

// BatchReplay wraps basic batch operations.
type BatchReplay interface {
	Put(key, value []byte)
	Delete(key []byte)
}

// Batch is a write batch.
type Batch struct {
	data       []byte
	rLen, bLen int
	seq        uint64
	sync       bool
}

func (b *Batch) grow(n int) {
	off := len(b.data)
	if off == 0 {
		off = batchHdrLen
		if b.data != nil {
			b.data = b.data[:off]
		}
	}
	if cap(b.data)-off < n {
		if b.data == nil {
			b.data = make([]byte, off, off+n)
		} else {
			odata := b.data
			div := 1
			if b.rLen > batchGrowRec {
				div = b.rLen / batchGrowRec
			}
			b.data = make([]byte, off, off+n+(off-batchHdrLen)/div)
			copy(b.data, odata)
		}
	}
}

func (b *Batch) appendRec(kt keyType, key, value []byte) {
	n := 1 + binary.MaxVarintLen32 + len(key)
	if kt == keyTypeVal {
		n += binary.MaxVarintLen32 + len(value)
	}
	b.grow(n)
	off := len(b.data)
	data := b.data[:off+n]
	data[off] = byte(kt)
	off++
	off += binary.PutUvarint(data[off:], uint64(len(key)))
	copy(data[off:], key)
	off += len(key)
	if kt == keyTypeVal {
		off += binary.PutUvarint(data[off:], uint64(len(value)))
		copy(data[off:], value)
		off += len(value)
	}
	b.data = data[:off]
	b.rLen++
	//  Include 8-byte ikey header
	b.bLen += len(key) + len(value) + 8
}

// Put appends 'put operation' of the given key/value pair to the batch.
// It is safe to modify the contents of the argument after Put returns.
func (b *Batch) Put(key, value []byte) {
	b.appendRec(keyTypeVal, key, value)
}

// Delete appends 'delete operation' of the given key to the batch.
// It is safe to modify the contents of the argument after Delete returns.
func (b *Batch) Delete(key []byte) {
	b.appendRec(keyTypeDel, key, nil)
}

// Dump dumps batch contents. The returned slice can be loaded into the
// batch using Load method.
// The returned slice is not its own copy, so the contents should not be
// modified.
func (b *Batch) Dump() []byte {
	return b.encode()
}

// Load loads given slice into the batch. Previous contents of the batch
// will be discarded.
// The given slice will not be copied and will be used as batch buffer, so
// it is not safe to modify the contents of the slice.
func (b *Batch) Load(data []byte) error {
	return b.decode(0, data)
}

// Replay replays batch contents.
func (b *Batch) Replay(r BatchReplay) error {
	return b.decodeRec(func(i int, kt keyType, key, value []byte) error {
		switch kt {
		case keyTypeVal:
			r.Put(key, value)
		case keyTypeDel:
			r.Delete(key)
		}
		return nil
	})
}

// Len returns number of records in the batch.
func (b *Batch) Len() int {
	return b.rLen
}

// Reset resets the batch.
func (b *Batch) Reset() {
	b.data = b.data[:0]
	b.seq = 0
	b.rLen = 0
	b.bLen = 0
	b.sync = false
}

func (b *Batch) init(sync bool) {
	b.sync = sync
}

func (b *Batch) append(p *Batch) {
	if p.rLen > 0 {
		b.grow(len(p.data) - batchHdrLen)
		b.data = append(b.data, p.data[batchHdrLen:]...)
		b.rLen += p.rLen
		b.bLen += p.bLen
	}
	if p.sync {
		b.sync = true
	}
}

// size returns sums of key/value pair length plus 8-bytes ikey.
func (b *Batch) size() int {
	return b.bLen
}

func (b *Batch) encode() []byte {
	b.grow(0)
	binary.LittleEndian.PutUint64(b.data, b.seq)
	binary.LittleEndian.PutUint32(b.data[8:], uint32(b.rLen))

	return b.data
}

func (b *Batch) decode(prevSeq uint64, data []byte) error {
	if len(data) < batchHdrLen {
		return newErrBatchCorrupted("too short")
	}

	b.seq = binary.LittleEndian.Uint64(data)
	if b.seq < prevSeq {
		return newErrBatchCorrupted("invalid sequence number")
	}
	b.rLen = int(binary.LittleEndian.Uint32(data[8:]))
	if b.rLen < 0 {
		return newErrBatchCorrupted("invalid records length")
	}
	// No need to be precise at this point, it won't be used anyway
	b.bLen = len(data) - batchHdrLen
	b.data = data

	return nil
}

func (b *Batch) decodeRec(f func(i int, kt keyType, key, value []byte) error) error {
	off := batchHdrLen
	for i := 0; i < b.rLen; i++ {
		if off >= len(b.data) {
			return newErrBatchCorrupted("invalid records length")
		}

		kt := keyType(b.data[off])
		if kt > keyTypeVal {
			panic(kt)
			return newErrBatchCorrupted("bad record: invalid type")
		}
		off++

		x, n := binary.Uvarint(b.data[off:])
		off += n
		if n <= 0 || off+int(x) > len(b.data) {
			return newErrBatchCorrupted("bad record: invalid key length")
		}
		key := b.data[off : off+int(x)]
		off += int(x)
		var value []byte
		if kt == keyTypeVal {
			x, n := binary.Uvarint(b.data[off:])
			off += n
			if n <= 0 || off+int(x) > len(b.data) {
				return newErrBatchCorrupted("bad record: invalid value length")
			}
			value = b.data[off : off+int(x)]
			off += int(x)
		}

		if err := f(i, kt, key, value); err != nil {
			return err
		}
	}

	return nil
}

func (b *Batch) memReplay(to *memdb.DB) error {
	var ikScratch []byte
	return b.decodeRec(func(i int, kt keyType, key, value []byte) error {
		ikScratch = makeInternalKey(ikScratch, key, b.seq+uint64(i), kt)
		return to.Put(ikScratch, value)
	})
}

func (b *Batch) memDecodeAndReplay(prevSeq uint64, data []byte, to *memdb.DB) error {
	if err := b.decode(prevSeq, data); err != nil {
		return err
	}
	return b.memReplay(to)
}

func (b *Batch) revertMemReplay(to *memdb.DB) error {
	var ikScratch []byte
	return b.decodeRec(func(i int, kt keyType, key, value []byte) error {
		ikScratch := makeInternalKey(ikScratch, key, b.seq+uint64(i), kt)
		return to.Delete(ikScratch)
	})
}
