// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb/memdb"
)

var (
	errCompactionTransactExiting = errors.New("leveldb: compaction transact exiting")
)

type cStats struct {
	sync.Mutex
	duration time.Duration
	read     uint64
	write    uint64
}

func (p *cStats) add(n *cStatsStaging) {
	p.Lock()
	p.duration += n.duration
	p.read += n.read
	p.write += n.write
	p.Unlock()
}

func (p *cStats) get() (duration time.Duration, read, write uint64) {
	p.Lock()
	defer p.Unlock()
	return p.duration, p.read, p.write
}

type cStatsStaging struct {
	start    time.Time
	duration time.Duration
	on       bool
	read     uint64
	write    uint64
}

func (p *cStatsStaging) startTimer() {
	if !p.on {
		p.start = time.Now()
		p.on = true
	}
}

func (p *cStatsStaging) stopTimer() {
	if p.on {
		p.duration += time.Since(p.start)
		p.on = false
	}
}

type cMem struct {
	s     *session
	level int
	rec   *sessionRecord
}

func newCMem(s *session) *cMem {
	return &cMem{s: s, rec: new(sessionRecord)}
}

func (c *cMem) flush(mem *memdb.DB, level int) error {
	s := c.s

	// Write memdb to table.
	iter := mem.NewIterator(nil)
	defer iter.Release()
	t, n, err := s.tops.createFrom(iter)
	if err != nil {
		return err
	}

	// Pick level.
	if level < 0 {
		level = s.version_NB().pickLevel(t.imin.ukey(), t.imax.ukey())
	}
	c.rec.addTableFile(level, t)

	s.logf("mem@flush created L%d@%d N·%d S·%s %q:%q", level, t.file.Num(), n, shortenb(int(t.size)), t.imin, t.imax)

	c.level = level
	return nil
}

func (c *cMem) reset() {
	c.rec = new(sessionRecord)
}

func (c *cMem) commit(journal, seq uint64) error {
	c.rec.setJournalNum(journal)
	c.rec.setSeq(seq)

	// Commit changes.
	return c.s.commit(c.rec)
}

func (db *DB) compactionError() {
	var err error
noerr:
	for {
		select {
		case err = <-db.compErrSetC:
			if err != nil {
				goto haserr
			}
		case _, _ = <-db.closeC:
			return
		}
	}
haserr:
	for {
		select {
		case db.compErrC <- err:
		case err = <-db.compErrSetC:
			if err == nil {
				goto noerr
			}
		case _, _ = <-db.closeC:
			return
		}
	}
}

type compactionTransactCounter int

func (cnt *compactionTransactCounter) incr() {
	*cnt++
}

func (db *DB) compactionTransact(name string, exec func(cnt *compactionTransactCounter) error, rollback func() error) {
	defer func() {
		if x := recover(); x != nil {
			if x == errCompactionTransactExiting && rollback != nil {
				if err := rollback(); err != nil {
					db.logf("%s rollback error %q", name, err)
				}
			}
			panic(x)
		}
	}()

	const (
		backoffMin = 1 * time.Second
		backoffMax = 8 * time.Second
		backoffMul = 2 * time.Second
	)
	backoff := backoffMin
	backoffT := time.NewTimer(backoff)
	lastCnt := compactionTransactCounter(0)
	for n := 0; ; n++ {
		// Check wether the DB is closed.
		if db.isClosed() {
			db.logf("%s exiting", name)
			db.compactionExitTransact()
		} else if n > 0 {
			db.logf("%s retrying N·%d", name, n)
		}

		// Execute.
		cnt := compactionTransactCounter(0)
		err := exec(&cnt)

		// Set compaction error status.
		select {
		case db.compErrSetC <- err:
		case _, _ = <-db.closeC:
			db.logf("%s exiting", name)
			db.compactionExitTransact()
		}
		if err == nil {
			return
		}
		db.logf("%s error I·%d %q", name, cnt, err)

		// Reset backoff duration if counter is advancing.
		if cnt > lastCnt {
			backoff = backoffMin
			lastCnt = cnt
		}

		// Backoff.
		backoffT.Reset(backoff)
		if backoff < backoffMax {
			backoff *= backoffMul
			if backoff > backoffMax {
				backoff = backoffMax
			}
		}
		select {
		case <-backoffT.C:
		case _, _ = <-db.closeC:
			db.logf("%s exiting", name)
			db.compactionExitTransact()
		}
	}
}

func (db *DB) compactionExitTransact() {
	panic(errCompactionTransactExiting)
}

func (db *DB) memCompaction() {
	mem := db.getFrozenMem()
	if mem == nil {
		return
	}
	defer mem.decref()

	c := newCMem(db.s)
	stats := new(cStatsStaging)

	db.logf("mem@flush N·%d S·%s", mem.mdb.Len(), shortenb(mem.mdb.Size()))

	// Don't compact empty memdb.
	if mem.mdb.Len() == 0 {
		db.logf("mem@flush skipping")
		// drop frozen mem
		db.dropFrozenMem()
		return
	}

	// Pause table compaction.
	ch := make(chan struct{})
	select {
	case db.tcompPauseC <- (chan<- struct{})(ch):
	case _, _ = <-db.closeC:
		return
	}

	db.compactionTransact("mem@flush", func(cnt *compactionTransactCounter) (err error) {
		stats.startTimer()
		defer stats.stopTimer()
		return c.flush(mem.mdb, -1)
	}, func() error {
		for _, r := range c.rec.addedTables {
			db.logf("mem@flush rollback @%d", r.num)
			f := db.s.getTableFile(r.num)
			if err := f.Remove(); err != nil {
				return err
			}
		}
		return nil
	})

	db.compactionTransact("mem@commit", func(cnt *compactionTransactCounter) (err error) {
		stats.startTimer()
		defer stats.stopTimer()
		return c.commit(db.journalFile.Num(), db.frozenSeq)
	}, nil)

	db.logf("mem@flush committed F·%d T·%v", len(c.rec.addedTables), stats.duration)

	for _, r := range c.rec.addedTables {
		stats.write += r.size
	}
	db.compStats[c.level].add(stats)

	// Drop frozen mem.
	db.dropFrozenMem()

	// Resume table compaction.
	select {
	case <-ch:
	case _, _ = <-db.closeC:
		return
	}

	// Trigger table compaction.
	db.compTrigger(db.mcompTriggerC)
}

func (db *DB) tableCompaction(c *compaction, noTrivial bool) {
	rec := new(sessionRecord)
	rec.addCompactionPointer(c.level, c.imax)

	if !noTrivial && c.trivial() {
		t := c.tables[0][0]
		db.logf("table@move L%d@%d -> L%d", c.level, t.file.Num(), c.level+1)
		rec.deleteTable(c.level, t.file.Num())
		rec.addTableFile(c.level+1, t)
		db.compactionTransact("table@move", func(cnt *compactionTransactCounter) (err error) {
			return db.s.commit(rec)
		}, nil)
		return
	}

	var stats [2]cStatsStaging
	for i, tables := range c.tables {
		for _, t := range tables {
			stats[i].read += t.size
			// Insert deleted tables into record
			rec.deleteTable(c.level+i, t.file.Num())
		}
	}
	sourceSize := int(stats[0].read + stats[1].read)
	minSeq := db.minSeq()
	db.logf("table@compaction L%d·%d -> L%d·%d S·%s Q·%d", c.level, len(c.tables[0]), c.level+1, len(c.tables[1]), shortenb(sourceSize), minSeq)

	var snapUkey []byte
	var snapHasUkey bool
	var snapSeq uint64
	var snapIter int
	var snapDropCnt int
	var dropCnt int
	db.compactionTransact("table@build", func(cnt *compactionTransactCounter) (err error) {
		ukey := append([]byte{}, snapUkey...)
		hasUkey := snapHasUkey
		lseq := snapSeq
		dropCnt = snapDropCnt
		snapSched := snapIter == 0

		var tw *tWriter
		finish := func() error {
			t, err := tw.finish()
			if err != nil {
				return err
			}
			rec.addTableFile(c.level+1, t)
			stats[1].write += t.size
			db.logf("table@build created L%d@%d N·%d S·%s %q:%q", c.level+1, t.file.Num(), tw.tw.EntriesLen(), shortenb(int(t.size)), t.imin, t.imax)
			return nil
		}

		defer func() {
			stats[1].stopTimer()
			if tw != nil {
				tw.drop()
				tw = nil
			}
		}()

		stats[1].startTimer()
		iter := c.newIterator()
		defer iter.Release()
		for i := 0; iter.Next(); i++ {
			// Incr transact counter.
			cnt.incr()

			// Skip until last state.
			if i < snapIter {
				continue
			}

			ikey := iKey(iter.Key())

			if c.shouldStopBefore(ikey) && tw != nil {
				err = finish()
				if err != nil {
					return
				}
				snapSched = true
				tw = nil
			}

			// Scheduled for snapshot, snapshot will used to retry compaction
			// if error occured.
			if snapSched {
				snapUkey = append(snapUkey[:0], ukey...)
				snapHasUkey = hasUkey
				snapSeq = lseq
				snapIter = i
				snapDropCnt = dropCnt
				snapSched = false
			}

			if seq, vt, ok := ikey.parseNum(); !ok {
				// Don't drop error keys
				ukey = ukey[:0]
				hasUkey = false
				lseq = kMaxSeq
			} else {
				if !hasUkey || db.s.icmp.uCompare(ikey.ukey(), ukey) != 0 {
					// First occurrence of this user key
					ukey = append(ukey[:0], ikey.ukey()...)
					hasUkey = true
					lseq = kMaxSeq
				}

				drop := false
				if lseq <= minSeq {
					// Dropped because newer entry for same user key exist
					drop = true // (A)
				} else if vt == tDel && seq <= minSeq && c.baseLevelForKey(ukey) {
					// For this user key:
					// (1) there is no data in higher levels
					// (2) data in lower levels will have larger seq numbers
					// (3) data in layers that are being compacted here and have
					//     smaller seq numbers will be dropped in the next
					//     few iterations of this loop (by rule (A) above).
					// Therefore this deletion marker is obsolete and can be dropped.
					drop = true
				}

				lseq = seq
				if drop {
					dropCnt++
					continue
				}
			}

			// Create new table if not already
			if tw == nil {
				// Check for pause event.
				select {
				case ch := <-db.tcompPauseC:
					db.pauseCompaction(ch)
				case _, _ = <-db.closeC:
					db.compactionExitTransact()
				default:
				}

				// Create new table.
				tw, err = db.s.tops.create()
				if err != nil {
					return
				}
			}

			// Write key/value into table
			err = tw.append(ikey, iter.Value())
			if err != nil {
				return
			}

			// Finish table if it is big enough
			if tw.tw.BytesLen() >= kMaxTableSize {
				err = finish()
				if err != nil {
					return
				}
				snapSched = true
				tw = nil
			}
		}

		err = iter.Error()
		if err != nil {
			return
		}

		// Finish last table
		if tw != nil && !tw.empty() {
			err = finish()
			if err != nil {
				return
			}
			tw = nil
		}
		return
	}, func() error {
		for _, r := range rec.addedTables {
			db.logf("table@build rollback @%d", r.num)
			f := db.s.getTableFile(r.num)
			if err := f.Remove(); err != nil {
				return err
			}
		}
		return nil
	})

	// Commit changes
	db.compactionTransact("table@commit", func(cnt *compactionTransactCounter) (err error) {
		stats[1].startTimer()
		defer stats[1].stopTimer()
		return db.s.commit(rec)
	}, nil)

	resultSize := int(stats[1].write)
	db.logf("table@compaction committed F%s S%s D·%d T·%v", sint(len(rec.addedTables)-len(rec.deletedTables)), sshortenb(resultSize-sourceSize), dropCnt, stats[1].duration)

	// Save compaction stats
	for i := range stats {
		db.compStats[c.level+1].add(&stats[i])
	}
}

func (db *DB) tableRangeCompaction(level int, umin, umax []byte) {
	db.logf("table@compaction range L%d %q:%q", level, umin, umax)

	if level >= 0 {
		if c := db.s.getCompactionRange(level, umin, umax); c != nil {
			db.tableCompaction(c, true)
		}
	} else {
		v := db.s.version_NB()

		m := 1
		for i, t := range v.tables[1:] {
			if t.overlaps(db.s.icmp, umin, umax, false) {
				m = i + 1
			}
		}

		for level := 0; level < m; level++ {
			if c := db.s.getCompactionRange(level, umin, umax); c != nil {
				db.tableCompaction(c, true)
			}
		}
	}
}

func (db *DB) tableAutoCompaction() {
	if c := db.s.pickCompaction(); c != nil {
		db.tableCompaction(c, false)
	}
}

func (db *DB) tableNeedCompaction() bool {
	return db.s.version_NB().needCompaction()
}

func (db *DB) pauseCompaction(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	case _, _ = <-db.closeC:
		db.compactionExitTransact()
	}
}

type cCmd interface {
	ack(err error)
}

type cIdle struct {
	ackC chan<- error
}

func (r cIdle) ack(err error) {
	r.ackC <- err
}

type cRange struct {
	level    int
	min, max []byte
	ackC     chan<- error
}

func (r cRange) ack(err error) {
	defer func() {
		recover()
	}()
	if r.ackC != nil {
		r.ackC <- err
	}
}

func (db *DB) compSendIdle(compC chan<- cCmd) error {
	ch := make(chan error)
	defer close(ch)
	// Send cmd.
	select {
	case compC <- cIdle{ch}:
	case err := <-db.compErrC:
		return err
	case _, _ = <-db.closeC:
		return ErrClosed
	}
	// Wait cmd.
	return <-ch
}

func (db *DB) compSendRange(compC chan<- cCmd, level int, min, max []byte) (err error) {
	ch := make(chan error)
	defer close(ch)
	// Send cmd.
	select {
	case compC <- cRange{level, min, max, ch}:
	case err := <-db.compErrC:
		return err
	case _, _ = <-db.closeC:
		return ErrClosed
	}
	// Wait cmd.
	select {
	case err = <-db.compErrC:
	case err = <-ch:
	}
	return err
}

func (db *DB) compTrigger(compTriggerC chan struct{}) {
	select {
	case compTriggerC <- struct{}{}:
	default:
	}
}

func (db *DB) mCompaction() {
	var x cCmd

	defer func() {
		if x := recover(); x != nil {
			if x != errCompactionTransactExiting {
				panic(x)
			}
		}
		if x != nil {
			x.ack(ErrClosed)
		}
		db.closeW.Done()
	}()

	for {
		select {
		case x = <-db.mcompCmdC:
			db.memCompaction()
			x.ack(nil)
			x = nil
		case <-db.mcompTriggerC:
			db.memCompaction()
		case _, _ = <-db.closeC:
			return
		}
	}
}

func (db *DB) tCompaction() {
	var x cCmd
	var ackQ []cCmd

	defer func() {
		if x := recover(); x != nil {
			if x != errCompactionTransactExiting {
				panic(x)
			}
		}
		for i := range ackQ {
			ackQ[i].ack(ErrClosed)
			ackQ[i] = nil
		}
		if x != nil {
			x.ack(ErrClosed)
		}
		db.closeW.Done()
	}()

	for {
		if db.tableNeedCompaction() {
			select {
			case x = <-db.tcompCmdC:
			case <-db.tcompTriggerC:
			case ch := <-db.tcompPauseC:
				db.pauseCompaction(ch)
				continue
			case _, _ = <-db.closeC:
				return
			default:
			}
		} else {
			for i := range ackQ {
				ackQ[i].ack(nil)
				ackQ[i] = nil
			}
			ackQ = ackQ[:0]
			select {
			case x = <-db.tcompCmdC:
			case <-db.tcompTriggerC:
			case ch := <-db.tcompPauseC:
				db.pauseCompaction(ch)
				continue
			case _, _ = <-db.closeC:
				return
			}
		}
		if x != nil {
			switch cmd := x.(type) {
			case cIdle:
				ackQ = append(ackQ, x)
			case cRange:
				db.tableRangeCompaction(cmd.level, cmd.min, cmd.max)
				x.ack(nil)
			}
			x = nil
		}
		db.tableAutoCompaction()
	}
}
