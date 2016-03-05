// Copyright (c) 2016, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var errTransactionDone = errors.New("leveldb: transaction already closed")

// Transaction is the transaction handle.
type Transaction struct {
	db        *DB
	lk        sync.RWMutex
	seq       uint64
	mem       *memDB
	tables    tFiles
	ikScratch []byte
	rec       sessionRecord
	stats     cStatStaging
	closed    bool
}

// Get gets the value for the given key. It returns ErrNotFound if the
// DB does not contains the key.
//
// The returned slice is its own copy, it is safe to modify the contents
// of the returned slice.
// It is safe to modify the contents of the argument after Get returns.
func (tr *Transaction) Get(key []byte, ro *opt.ReadOptions) ([]byte, error) {
	tr.lk.RLock()
	defer tr.lk.RUnlock()
	if tr.closed {
		return nil, errTransactionDone
	}
	return tr.db.get(tr.mem.DB, tr.tables, key, tr.seq, ro)
}

// Has returns true if the DB does contains the given key.
//
// It is safe to modify the contents of the argument after Has returns.
func (tr *Transaction) Has(key []byte, ro *opt.ReadOptions) (bool, error) {
	tr.lk.RLock()
	defer tr.lk.RUnlock()
	if tr.closed {
		return false, errTransactionDone
	}
	return tr.db.has(tr.mem.DB, tr.tables, key, tr.seq, ro)
}

// NewIterator returns an iterator for the latest snapshot of the transaction.
// The returned iterator is not goroutine-safe, but it is safe to use multiple
// iterators concurrently, with each in a dedicated goroutine.
// It is also safe to use an iterator concurrently while writes to the
// transaction. The resultant key/value pairs are guaranteed to be consistent.
//
// Slice allows slicing the iterator to only contains keys in the given
// range. A nil Range.Start is treated as a key before all keys in the
// DB. And a nil Range.Limit is treated as a key after all keys in
// the DB.
//
// The iterator must be released after use, by calling Release method.
//
// Also read Iterator documentation of the leveldb/iterator package.
func (tr *Transaction) NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	tr.lk.RLock()
	defer tr.lk.RUnlock()
	if tr.closed {
		return iterator.NewEmptyIterator(errTransactionDone)
	}
	tr.mem.incref()
	return tr.db.newIterator(tr.mem, tr.tables, tr.seq, slice, ro)
}

func (tr *Transaction) flush() error {
	// Flush memdb.
	if tr.mem.Len() != 0 {
		tr.stats.startTimer()
		iter := tr.mem.NewIterator(nil)
		t, n, err := tr.db.s.tops.createFrom(iter)
		iter.Release()
		tr.stats.stopTimer()
		if err != nil {
			return err
		}
		if tr.mem.getref() == 1 {
			tr.mem.Reset()
		} else {
			tr.mem.decref()
			tr.mem = tr.db.mpoolGet(0)
			tr.mem.incref()
		}
		tr.tables = append(tr.tables, t)
		tr.rec.addTableFile(0, t)
		tr.stats.write += t.size
		tr.db.logf("transaction@flush created L0@%d N·%d S·%s %q:%q", t.fd.Num, n, shortenb(int(t.size)), t.imin, t.imax)
	}
	return nil
}

func (tr *Transaction) put(kt keyType, key, value []byte) error {
	tr.ikScratch = makeInternalKey(tr.ikScratch, key, tr.seq+1, kt)
	if tr.mem.Free() < len(tr.ikScratch)+len(value) {
		if err := tr.flush(); err != nil {
			return err
		}
	}
	if err := tr.mem.Put(tr.ikScratch, value); err != nil {
		return err
	}
	tr.seq++
	return nil
}

// Put sets the value for the given key. It overwrites any previous value
// for that key; a DB is not a multi-map.
// Please note that the transaction is not compacted until committed, so if you
// writes 10 same keys, then those 10 same keys are in the transaction.
//
// It is safe to modify the contents of the arguments after Put returns.
func (tr *Transaction) Put(key, value []byte, wo *opt.WriteOptions) error {
	tr.lk.Lock()
	defer tr.lk.Unlock()
	if tr.closed {
		return errTransactionDone
	}
	return tr.put(keyTypeVal, key, value)
}

// Delete deletes the value for the given key.
// Please note that the transaction is not compacted until committed, so if you
// writes 10 same keys, then those 10 same keys are in the transaction.
//
// It is safe to modify the contents of the arguments after Delete returns.
func (tr *Transaction) Delete(key []byte, wo *opt.WriteOptions) error {
	tr.lk.Lock()
	defer tr.lk.Unlock()
	if tr.closed {
		return errTransactionDone
	}
	return tr.put(keyTypeDel, key, nil)
}

// Write apply the given batch to the transaction. The batch will be applied
// sequentially.
// Please note that the transaction is not compacted until committed, so if you
// writes 10 same keys, then those 10 same keys are in the transaction.
//
// It is safe to modify the contents of the arguments after Write returns.
func (tr *Transaction) Write(b *Batch, wo *opt.WriteOptions) error {
	if b == nil || b.Len() == 0 {
		return nil
	}

	tr.lk.Lock()
	defer tr.lk.Unlock()
	if tr.closed {
		return errTransactionDone
	}
	return b.decodeRec(func(i int, kt keyType, key, value []byte) error {
		return tr.put(kt, key, value)
	})
}

func (tr *Transaction) setDone() {
	tr.closed = true
	tr.db.tr = nil
	tr.mem.decref()
	<-tr.db.writeLockC
}

// Commit commits the transaction.
//
// Other methods should not be called after transaction has been committed.
func (tr *Transaction) Commit() error {
	if err := tr.db.ok(); err != nil {
		return err
	}

	tr.lk.Lock()
	defer tr.lk.Unlock()
	if tr.closed {
		return errTransactionDone
	}
	defer tr.setDone()
	if err := tr.flush(); err != nil {
		tr.discard()
		return err
	}
	if len(tr.tables) != 0 {
		// Committing transaction.
		tr.rec.setSeqNum(tr.seq)
		tr.db.compCommitLk.Lock()
		defer tr.db.compCommitLk.Unlock()
		for retry := 0; retry < 3; retry++ {
			if err := tr.db.s.commit(&tr.rec); err != nil {
				tr.db.logf("transaction@commit error R·%d %q", retry, err)
				select {
				case <-time.After(time.Second):
				case _, _ = <-tr.db.closeC:
					tr.db.logf("transaction@commit exiting")
					return err
				}
			} else {
				// Success. Set db.seq.
				tr.db.setSeq(tr.seq)
				break
			}
		}
		// Trigger table auto-compaction.
		tr.db.compTrigger(tr.db.tcompCmdC)
	}
	return nil
}

func (tr *Transaction) discard() {
	// Discard transaction.
	for _, t := range tr.tables {
		tr.db.logf("transaction@discard @%d", t.fd.Num)
		if err1 := tr.db.s.stor.Remove(t.fd); err1 == nil {
			tr.db.s.reuseFileNum(t.fd.Num)
		}
	}
}

// Discard discards the transaction.
//
// Other methods should not be called after transaction has been discarded.
func (tr *Transaction) Discard() {
	tr.lk.Lock()
	if !tr.closed {
		tr.discard()
		tr.setDone()
	}
	tr.lk.Unlock()
}

// OpenTransaction opens an atomic DB transaction. Only one transaction can be
// opened at a time. Write will be blocked until the transaction is committed or
// discarded.
// The returned transaction handle is goroutine-safe.
//
// The transaction must be closed once done, either by committing or discarding
// the transaction.
// Closing the DB will discard open transaction.
func (db *DB) OpenTransaction() (*Transaction, error) {
	if err := db.ok(); err != nil {
		return nil, err
	}

	// The write happen synchronously.
	select {
	case db.writeLockC <- struct{}{}:
	case err := <-db.compPerErrC:
		return nil, err
	case _, _ = <-db.closeC:
		return nil, ErrClosed
	}

	if db.tr != nil {
		panic("leveldb: has open transaction")
	}

	// Flush current memdb.
	if db.mem != nil && db.mem.Len() != 0 {
		if _, err := db.rotateMem(0, true); err != nil {
			return nil, err
		}
	}

	tr := &Transaction{
		db:  db,
		seq: db.seq,
		mem: db.mpoolGet(0),
	}
	tr.mem.incref()
	db.tr = tr
	return tr, nil
}
