// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/memdb"
)

// Get latest sequence number.
func (d *DB) getSeq() uint64 {
	return atomic.LoadUint64(&d.seq)
}

// Atomically adds delta to seq.
func (d *DB) addSeq(delta uint64) {
	atomic.AddUint64(&d.seq, delta)
}

// Create new memdb and froze the old one; need external synchronization.
// newMem only called synchronously by the writer.
func (d *DB) newMem(n int) (mem *memdb.DB, err error) {
	s := d.s

	num := s.allocFileNum()
	file := s.getJournalFile(num)
	w, err := file.Create()
	if err != nil {
		s.reuseFileNum(num)
		return
	}
	d.memMu.Lock()
	if d.journal == nil {
		d.journal = journal.NewWriter(w)
	} else {
		d.journal.Reset(w)
		d.journalWriter.Close()
		d.frozenJournalFile = d.journalFile
	}
	d.journalWriter = w
	d.journalFile = file
	d.frozenMem = d.mem
	d.mem = memdb.New(s.icmp, maxInt(d.s.o.GetWriteBuffer(), n))
	mem = d.mem
	// The seq only incremented by the writer.
	d.frozenSeq = d.seq
	d.memMu.Unlock()
	return
}

// Get all memdbs.
func (d *DB) getMems() (e *memdb.DB, f *memdb.DB) {
	d.memMu.RLock()
	defer d.memMu.RUnlock()
	return d.mem, d.frozenMem
}

// Get frozen memdb.
func (d *DB) getEffectiveMem() *memdb.DB {
	d.memMu.RLock()
	defer d.memMu.RUnlock()
	return d.mem
}

// Check whether we has frozen memdb.
func (d *DB) hasFrozenMem() bool {
	d.memMu.RLock()
	defer d.memMu.RUnlock()
	return d.frozenMem != nil
}

// Get frozen memdb.
func (d *DB) getFrozenMem() *memdb.DB {
	d.memMu.RLock()
	defer d.memMu.RUnlock()
	return d.frozenMem
}

// Drop frozen memdb; assume that frozen memdb isn't nil.
func (d *DB) dropFrozenMem() {
	d.memMu.Lock()
	if err := d.frozenJournalFile.Remove(); err != nil {
		d.s.logf("journal@remove removing @%d %q", d.frozenJournalFile.Num(), err)
	} else {
		d.s.logf("journal@remove removed @%d", d.frozenJournalFile.Num())
	}
	d.frozenJournalFile = nil
	d.frozenMem = nil
	d.memMu.Unlock()
}

// Set closed flag; return true if not already closed.
func (d *DB) setClosed() bool {
	return atomic.CompareAndSwapUint32(&d.closed, 0, 1)
}

// Check whether DB was closed.
func (d *DB) isClosed() bool {
	return atomic.LoadUint32(&d.closed) != 0
}

// Check read ok status.
func (d *DB) ok() error {
	if d.isClosed() {
		return ErrClosed
	}
	return nil
}
