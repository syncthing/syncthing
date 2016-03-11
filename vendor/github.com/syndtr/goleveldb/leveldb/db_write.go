// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"time"

	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func (db *DB) writeJournal(b *Batch) error {
	w, err := db.journal.Next()
	if err != nil {
		return err
	}
	if _, err := w.Write(b.encode()); err != nil {
		return err
	}
	if err := db.journal.Flush(); err != nil {
		return err
	}
	if b.sync {
		return db.journalWriter.Sync()
	}
	return nil
}

func (db *DB) jWriter() {
	defer db.closeW.Done()
	for {
		select {
		case b := <-db.journalC:
			if b != nil {
				db.journalAckC <- db.writeJournal(b)
			}
		case _, _ = <-db.closeC:
			return
		}
	}
}

func (db *DB) rotateMem(n int, wait bool) (mem *memDB, err error) {
	// Wait for pending memdb compaction.
	err = db.compTriggerWait(db.mcompCmdC)
	if err != nil {
		return
	}

	// Create new memdb and journal.
	mem, err = db.newMem(n)
	if err != nil {
		return
	}

	// Schedule memdb compaction.
	if wait {
		err = db.compTriggerWait(db.mcompCmdC)
	} else {
		db.compTrigger(db.mcompCmdC)
	}
	return
}

func (db *DB) flush(n int) (mdb *memDB, mdbFree int, err error) {
	delayed := false
	flush := func() (retry bool) {
		v := db.s.version()
		defer v.release()
		mdb = db.getEffectiveMem()
		defer func() {
			if retry {
				mdb.decref()
				mdb = nil
			}
		}()
		mdbFree = mdb.Free()
		switch {
		case v.tLen(0) >= db.s.o.GetWriteL0SlowdownTrigger() && !delayed:
			delayed = true
			time.Sleep(time.Millisecond)
		case mdbFree >= n:
			return false
		case v.tLen(0) >= db.s.o.GetWriteL0PauseTrigger():
			delayed = true
			err = db.compTriggerWait(db.tcompCmdC)
			if err != nil {
				return false
			}
		default:
			// Allow memdb to grow if it has no entry.
			if mdb.Len() == 0 {
				mdbFree = n
			} else {
				mdb.decref()
				mdb, err = db.rotateMem(n, false)
				if err == nil {
					mdbFree = mdb.Free()
				} else {
					mdbFree = 0
				}
			}
			return false
		}
		return true
	}
	start := time.Now()
	for flush() {
	}
	if delayed {
		db.writeDelay += time.Since(start)
		db.writeDelayN++
	} else if db.writeDelayN > 0 {
		db.logf("db@write was delayed N·%d T·%v", db.writeDelayN, db.writeDelay)
		db.writeDelay = 0
		db.writeDelayN = 0
	}
	return
}

// Write apply the given batch to the DB. The batch will be applied
// sequentially.
//
// It is safe to modify the contents of the arguments after Write returns.
func (db *DB) Write(b *Batch, wo *opt.WriteOptions) (err error) {
	err = db.ok()
	if err != nil || b == nil || b.Len() == 0 {
		return
	}

	b.init(wo.GetSync() && !db.s.o.GetNoSync())

	if b.size() > db.s.o.GetWriteBuffer() && !db.s.o.GetDisableLargeBatchTransaction() {
		// Writes using transaction.
		tr, err1 := db.OpenTransaction()
		if err1 != nil {
			return err1
		}
		if err1 := tr.Write(b, wo); err1 != nil {
			tr.Discard()
			return err1
		}
		return tr.Commit()
	}

	// The write happen synchronously.
	select {
	case db.writeC <- b:
		if <-db.writeMergedC {
			return <-db.writeAckC
		}
		// Continue, the write lock already acquired by previous writer
		// and handed out to us.
	case db.writeLockC <- struct{}{}:
	case err = <-db.compPerErrC:
		return
	case _, _ = <-db.closeC:
		return ErrClosed
	}

	merged := 0
	danglingMerge := false
	defer func() {
		for i := 0; i < merged; i++ {
			db.writeAckC <- err
		}
		if danglingMerge {
			// Only one dangling merge at most, so this is safe.
			db.writeMergedC <- false
		} else {
			<-db.writeLockC
		}
	}()

	mdb, mdbFree, err := db.flush(b.size())
	if err != nil {
		return
	}
	defer mdb.decref()

	// Calculate maximum size of the batch.
	m := 1 << 20
	if x := b.size(); x <= 128<<10 {
		m = x + (128 << 10)
	}
	m = minInt(m, mdbFree)

	// Merge with other batch.
drain:
	for b.size() < m && !b.sync {
		select {
		case nb := <-db.writeC:
			if b.size()+nb.size() <= m {
				b.append(nb)
				db.writeMergedC <- true
				merged++
			} else {
				danglingMerge = true
				break drain
			}
		default:
			break drain
		}
	}

	// Set batch first seq number relative from last seq.
	b.seq = db.seq + 1

	// Write journal concurrently if it is large enough.
	if b.size() >= (128 << 10) {
		// Push the write batch to the journal writer
		select {
		case db.journalC <- b:
			// Write into memdb
			if berr := b.memReplay(mdb.DB); berr != nil {
				panic(berr)
			}
		case err = <-db.compPerErrC:
			return
		case _, _ = <-db.closeC:
			err = ErrClosed
			return
		}
		// Wait for journal writer
		select {
		case err = <-db.journalAckC:
			if err != nil {
				// Revert memdb if error detected
				if berr := b.revertMemReplay(mdb.DB); berr != nil {
					panic(berr)
				}
				return
			}
		case _, _ = <-db.closeC:
			err = ErrClosed
			return
		}
	} else {
		err = db.writeJournal(b)
		if err != nil {
			return
		}
		if berr := b.memReplay(mdb.DB); berr != nil {
			panic(berr)
		}
	}

	// Set last seq number.
	db.addSeq(uint64(b.Len()))

	if b.size() >= mdbFree {
		db.rotateMem(0, false)
	}
	return
}

// Put sets the value for the given key. It overwrites any previous value
// for that key; a DB is not a multi-map.
//
// It is safe to modify the contents of the arguments after Put returns.
func (db *DB) Put(key, value []byte, wo *opt.WriteOptions) error {
	b := new(Batch)
	b.Put(key, value)
	return db.Write(b, wo)
}

// Delete deletes the value for the given key.
//
// It is safe to modify the contents of the arguments after Delete returns.
func (db *DB) Delete(key []byte, wo *opt.WriteOptions) error {
	b := new(Batch)
	b.Delete(key)
	return db.Write(b, wo)
}

func isMemOverlaps(icmp *iComparer, mem *memdb.DB, min, max []byte) bool {
	iter := mem.NewIterator(nil)
	defer iter.Release()
	return (max == nil || (iter.First() && icmp.uCompare(max, internalKey(iter.Key()).ukey()) >= 0)) &&
		(min == nil || (iter.Last() && icmp.uCompare(min, internalKey(iter.Key()).ukey()) <= 0))
}

// CompactRange compacts the underlying DB for the given key range.
// In particular, deleted and overwritten versions are discarded,
// and the data is rearranged to reduce the cost of operations
// needed to access the data. This operation should typically only
// be invoked by users who understand the underlying implementation.
//
// A nil Range.Start is treated as a key before all keys in the DB.
// And a nil Range.Limit is treated as a key after all keys in the DB.
// Therefore if both is nil then it will compact entire DB.
func (db *DB) CompactRange(r util.Range) error {
	if err := db.ok(); err != nil {
		return err
	}

	// Lock writer.
	select {
	case db.writeLockC <- struct{}{}:
	case err := <-db.compPerErrC:
		return err
	case _, _ = <-db.closeC:
		return ErrClosed
	}

	// Check for overlaps in memdb.
	mdb := db.getEffectiveMem()
	defer mdb.decref()
	if isMemOverlaps(db.s.icmp, mdb.DB, r.Start, r.Limit) {
		// Memdb compaction.
		if _, err := db.rotateMem(0, false); err != nil {
			<-db.writeLockC
			return err
		}
		<-db.writeLockC
		if err := db.compTriggerWait(db.mcompCmdC); err != nil {
			return err
		}
	} else {
		<-db.writeLockC
	}

	// Table compaction.
	return db.compTriggerRange(db.tcompCmdC, -1, r.Start, r.Limit)
}

// SetReadOnly makes DB read-only. It will stay read-only until reopened.
func (db *DB) SetReadOnly() error {
	if err := db.ok(); err != nil {
		return err
	}

	// Lock writer.
	select {
	case db.writeLockC <- struct{}{}:
		db.compWriteLocking = true
	case err := <-db.compPerErrC:
		return err
	case _, _ = <-db.closeC:
		return ErrClosed
	}

	// Set compaction read-only.
	select {
	case db.compErrSetC <- ErrReadOnly:
	case perr := <-db.compPerErrC:
		return perr
	case _, _ = <-db.closeC:
		return ErrClosed
	}

	return nil
}
