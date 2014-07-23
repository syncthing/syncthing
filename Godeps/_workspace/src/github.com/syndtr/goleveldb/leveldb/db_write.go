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

func (db *DB) rotateMem(n int) (mem *memdb.DB, err error) {
	// Wait for pending memdb compaction.
	err = db.compSendIdle(db.mcompCmdC)
	if err != nil {
		return
	}

	// Create new memdb and journal.
	mem, err = db.newMem(n)
	if err != nil {
		return
	}

	// Schedule memdb compaction.
	db.compTrigger(db.mcompTriggerC)
	return
}

func (db *DB) flush(n int) (mem *memdb.DB, nn int, err error) {
	delayed := false
	flush := func() bool {
		v := db.s.version()
		defer v.release()
		mem = db.getEffectiveMem()
		nn = mem.Free()
		switch {
		case v.tLen(0) >= kL0_SlowdownWritesTrigger && !delayed:
			delayed = true
			time.Sleep(time.Millisecond)
		case nn >= n:
			return false
		case v.tLen(0) >= kL0_StopWritesTrigger:
			delayed = true
			err = db.compSendIdle(db.tcompCmdC)
			if err != nil {
				return false
			}
		default:
			// Allow memdb to grow if it has no entry.
			if mem.Len() == 0 {
				nn = n
				return false
			}
			mem, err = db.rotateMem(n)
			nn = mem.Free()
			return false
		}
		return true
	}
	start := time.Now()
	for flush() {
	}
	if delayed {
		db.logf("db@write delayed T·%v", time.Since(start))
	}
	return
}

// Write apply the given batch to the DB. The batch will be applied
// sequentially.
//
// It is safe to modify the contents of the arguments after Write returns.
func (db *DB) Write(b *Batch, wo *opt.WriteOptions) (err error) {
	err = db.ok()
	if err != nil || b == nil || b.len() == 0 {
		return
	}

	b.init(wo.GetSync())

	// The write happen synchronously.
retry:
	select {
	case db.writeC <- b:
		if <-db.writeMergedC {
			return <-db.writeAckC
		}
		goto retry
	case db.writeLockC <- struct{}{}:
	case _, _ = <-db.closeC:
		return ErrClosed
	}

	merged := 0
	defer func() {
		<-db.writeLockC
		for i := 0; i < merged; i++ {
			db.writeAckC <- err
		}
	}()

	mem, memFree, err := db.flush(b.size())
	if err != nil {
		return
	}

	// Calculate maximum size of the batch.
	m := 1 << 20
	if x := b.size(); x <= 128<<10 {
		m = x + (128 << 10)
	}
	m = minInt(m, memFree)

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
				db.writeMergedC <- false
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
		case _, _ = <-db.closeC:
			err = ErrClosed
			return
		case db.journalC <- b:
			// Write into memdb
			b.memReplay(mem)
		}
		// Wait for journal writer
		select {
		case _, _ = <-db.closeC:
			err = ErrClosed
			return
		case err = <-db.journalAckC:
			if err != nil {
				// Revert memdb if error detected
				b.revertMemReplay(mem)
				return
			}
		}
	} else {
		err = db.writeJournal(b)
		if err != nil {
			return
		}
		b.memReplay(mem)
	}

	// Set last seq number.
	db.addSeq(uint64(b.len()))

	if b.size() >= memFree {
		db.rotateMem(0)
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

// Delete deletes the value for the given key. It returns ErrNotFound if
// the DB does not contain the key.
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
	return (max == nil || (iter.First() && icmp.uCompare(max, iKey(iter.Key()).ukey()) >= 0)) &&
		(min == nil || (iter.Last() && icmp.uCompare(min, iKey(iter.Key()).ukey()) <= 0))
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

	select {
	case db.writeLockC <- struct{}{}:
	case _, _ = <-db.closeC:
		return ErrClosed
	}

	// Check for overlaps in memdb.
	mem := db.getEffectiveMem()
	if isMemOverlaps(db.s.icmp, mem, r.Start, r.Limit) {
		// Memdb compaction.
		if _, err := db.rotateMem(0); err != nil {
			<-db.writeLockC
			return err
		}
		<-db.writeLockC
		if err := db.compSendIdle(db.mcompCmdC); err != nil {
			return err
		}
	} else {
		<-db.writeLockC
	}

	// Table compaction.
	return db.compSendRange(db.tcompCmdC, -1, r.Start, r.Limit)
}
