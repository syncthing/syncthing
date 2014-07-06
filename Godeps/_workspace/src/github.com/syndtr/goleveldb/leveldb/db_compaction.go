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

	// Write memdb to table
	iter := mem.NewIterator(nil)
	defer iter.Release()
	t, n, err := s.tops.createFrom(iter)
	if err != nil {
		return err
	}

	if level < 0 {
		level = s.version_NB().pickLevel(t.min.ukey(), t.max.ukey())
	}
	c.rec.addTableFile(level, t)

	s.logf("mem@flush created L%d@%d N·%d S·%s %q:%q", level, t.file.Num(), n, shortenb(int(t.size)), t.min, t.max)

	c.level = level
	return nil
}

func (c *cMem) reset() {
	c.rec = new(sessionRecord)
}

func (c *cMem) commit(journal, seq uint64) error {
	c.rec.setJournalNum(journal)
	c.rec.setSeq(seq)
	// Commit changes
	return c.s.commit(c.rec)
}

func (d *DB) compactionError() {
	var err error
noerr:
	for {
		select {
		case _, _ = <-d.closeC:
			return
		case err = <-d.compErrSetC:
			if err != nil {
				goto haserr
			}
		}
	}
haserr:
	for {
		select {
		case _, _ = <-d.closeC:
			return
		case err = <-d.compErrSetC:
			if err == nil {
				goto noerr
			}
		case d.compErrC <- err:
		}
	}
}

type compactionTransactCounter int

func (cnt *compactionTransactCounter) incr() {
	*cnt++
}

func (d *DB) compactionTransact(name string, exec func(cnt *compactionTransactCounter) error, rollback func() error) {
	s := d.s
	defer func() {
		if x := recover(); x != nil {
			if x == errCompactionTransactExiting && rollback != nil {
				if err := rollback(); err != nil {
					s.logf("%s rollback error %q", name, err)
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
		if d.isClosed() {
			s.logf("%s exiting", name)
			d.compactionExitTransact()
		} else if n > 0 {
			s.logf("%s retrying N·%d", name, n)
		}

		// Execute.
		cnt := compactionTransactCounter(0)
		err := exec(&cnt)

		// Set compaction error status.
		select {
		case d.compErrSetC <- err:
		case _, _ = <-d.closeC:
			s.logf("%s exiting", name)
			d.compactionExitTransact()
		}
		if err == nil {
			return
		}
		s.logf("%s error I·%d %q", name, cnt, err)

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
		case _, _ = <-d.closeC:
			s.logf("%s exiting", name)
			d.compactionExitTransact()
		}
	}
}

func (d *DB) compactionExitTransact() {
	panic(errCompactionTransactExiting)
}

func (d *DB) memCompaction() {
	mem := d.getFrozenMem()
	if mem == nil {
		return
	}

	s := d.s
	c := newCMem(s)
	stats := new(cStatsStaging)

	s.logf("mem@flush N·%d S·%s", mem.Len(), shortenb(mem.Size()))

	// Don't compact empty memdb.
	if mem.Len() == 0 {
		s.logf("mem@flush skipping")
		// drop frozen mem
		d.dropFrozenMem()
		return
	}

	// Pause table compaction.
	ch := make(chan struct{})
	select {
	case d.tcompPauseC <- (chan<- struct{})(ch):
	case _, _ = <-d.closeC:
		return
	}

	d.compactionTransact("mem@flush", func(cnt *compactionTransactCounter) (err error) {
		stats.startTimer()
		defer stats.stopTimer()
		return c.flush(mem, -1)
	}, func() error {
		for _, r := range c.rec.addedTables {
			s.logf("mem@flush rollback @%d", r.num)
			f := s.getTableFile(r.num)
			if err := f.Remove(); err != nil {
				return err
			}
		}
		return nil
	})

	d.compactionTransact("mem@commit", func(cnt *compactionTransactCounter) (err error) {
		stats.startTimer()
		defer stats.stopTimer()
		return c.commit(d.journalFile.Num(), d.frozenSeq)
	}, nil)

	s.logf("mem@flush commited F·%d T·%v", len(c.rec.addedTables), stats.duration)

	for _, r := range c.rec.addedTables {
		stats.write += r.size
	}
	d.compStats[c.level].add(stats)

	// Drop frozen mem.
	d.dropFrozenMem()

	// Resume table compaction.
	select {
	case <-ch:
	case _, _ = <-d.closeC:
		return
	}

	// Trigger table compaction.
	d.compTrigger(d.mcompTriggerC)
}

func (d *DB) tableCompaction(c *compaction, noTrivial bool) {
	s := d.s

	rec := new(sessionRecord)
	rec.addCompactionPointer(c.level, c.max)

	if !noTrivial && c.trivial() {
		t := c.tables[0][0]
		s.logf("table@move L%d@%d -> L%d", c.level, t.file.Num(), c.level+1)
		rec.deleteTable(c.level, t.file.Num())
		rec.addTableFile(c.level+1, t)
		d.compactionTransact("table@move", func(cnt *compactionTransactCounter) (err error) {
			return s.commit(rec)
		}, nil)
		return
	}

	var stats [2]cStatsStaging
	for i, tt := range c.tables {
		for _, t := range tt {
			stats[i].read += t.size
			// Insert deleted tables into record
			rec.deleteTable(c.level+i, t.file.Num())
		}
	}
	sourceSize := int(stats[0].read + stats[1].read)
	minSeq := d.minSeq()
	s.logf("table@compaction L%d·%d -> L%d·%d S·%s Q·%d", c.level, len(c.tables[0]), c.level+1, len(c.tables[1]), shortenb(sourceSize), minSeq)

	var snapUkey []byte
	var snapHasUkey bool
	var snapSeq uint64
	var snapIter int
	var snapDropCnt int
	var dropCnt int
	d.compactionTransact("table@build", func(cnt *compactionTransactCounter) (err error) {
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
			s.logf("table@build created L%d@%d N·%d S·%s %q:%q", c.level+1, t.file.Num(), tw.tw.EntriesLen(), shortenb(int(t.size)), t.min, t.max)
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

			key := iKey(iter.Key())

			if c.shouldStopBefore(key) && tw != nil {
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

			if seq, t, ok := key.parseNum(); !ok {
				// Don't drop error keys
				ukey = ukey[:0]
				hasUkey = false
				lseq = kMaxSeq
			} else {
				if !hasUkey || s.icmp.uCompare(key.ukey(), ukey) != 0 {
					// First occurrence of this user key
					ukey = append(ukey[:0], key.ukey()...)
					hasUkey = true
					lseq = kMaxSeq
				}

				drop := false
				if lseq <= minSeq {
					// Dropped because newer entry for same user key exist
					drop = true // (A)
				} else if t == tDel && seq <= minSeq && c.isBaseLevelForKey(ukey) {
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
				case ch := <-d.tcompPauseC:
					d.pauseCompaction(ch)
				case _, _ = <-d.closeC:
					d.compactionExitTransact()
				default:
				}

				// Create new table.
				tw, err = s.tops.create()
				if err != nil {
					return
				}
			}

			// Write key/value into table
			err = tw.add(key, iter.Value())
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
			s.logf("table@build rollback @%d", r.num)
			f := s.getTableFile(r.num)
			if err := f.Remove(); err != nil {
				return err
			}
		}
		return nil
	})

	// Commit changes
	d.compactionTransact("table@commit", func(cnt *compactionTransactCounter) (err error) {
		stats[1].startTimer()
		defer stats[1].stopTimer()
		return s.commit(rec)
	}, nil)

	resultSize := int(int(stats[1].write))
	s.logf("table@compaction commited F%s S%s D·%d T·%v", sint(len(rec.addedTables)-len(rec.deletedTables)), sshortenb(resultSize-sourceSize), dropCnt, stats[1].duration)

	// Save compaction stats
	for i := range stats {
		d.compStats[c.level+1].add(&stats[i])
	}
}

func (d *DB) tableRangeCompaction(level int, min, max []byte) {
	s := d.s
	s.logf("table@compaction range L%d %q:%q", level, min, max)

	if level >= 0 {
		if c := s.getCompactionRange(level, min, max); c != nil {
			d.tableCompaction(c, true)
		}
	} else {
		v := s.version_NB()
		m := 1
		for i, t := range v.tables[1:] {
			if t.isOverlaps(min, max, true, s.icmp) {
				m = i + 1
			}
		}
		for level := 0; level < m; level++ {
			if c := s.getCompactionRange(level, min, max); c != nil {
				d.tableCompaction(c, true)
			}
		}
	}
}

func (d *DB) tableAutoCompaction() {
	if c := d.s.pickCompaction(); c != nil {
		d.tableCompaction(c, false)
	}
}

func (d *DB) tableNeedCompaction() bool {
	return d.s.version_NB().needCompaction()
}

func (d *DB) pauseCompaction(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	case _, _ = <-d.closeC:
		d.compactionExitTransact()
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

func (d *DB) compSendIdle(compC chan<- cCmd) error {
	ch := make(chan error)
	defer close(ch)
	// Send cmd.
	select {
	case compC <- cIdle{ch}:
	case err := <-d.compErrC:
		return err
	case _, _ = <-d.closeC:
		return ErrClosed
	}
	// Wait cmd.
	return <-ch
}

func (d *DB) compSendRange(compC chan<- cCmd, level int, min, max []byte) (err error) {
	ch := make(chan error)
	defer close(ch)
	// Send cmd.
	select {
	case compC <- cRange{level, min, max, ch}:
	case err := <-d.compErrC:
		return err
	case _, _ = <-d.closeC:
		return ErrClosed
	}
	// Wait cmd.
	select {
	case err = <-d.compErrC:
	case err = <-ch:
	}
	return err
}

func (d *DB) compTrigger(compTriggerC chan struct{}) {
	select {
	case compTriggerC <- struct{}{}:
	default:
	}
}

func (d *DB) mCompaction() {
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
		d.closeW.Done()
	}()

	for {
		select {
		case _, _ = <-d.closeC:
			return
		case x = <-d.mcompCmdC:
			d.memCompaction()
			x.ack(nil)
			x = nil
		case <-d.mcompTriggerC:
			d.memCompaction()
		}
	}
}

func (d *DB) tCompaction() {
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
		d.closeW.Done()
	}()

	for {
		if d.tableNeedCompaction() {
			select {
			case x = <-d.tcompCmdC:
			case <-d.tcompTriggerC:
			case _, _ = <-d.closeC:
				return
			case ch := <-d.tcompPauseC:
				d.pauseCompaction(ch)
				continue
			default:
			}
		} else {
			for i := range ackQ {
				ackQ[i].ack(nil)
				ackQ[i] = nil
			}
			ackQ = ackQ[:0]
			select {
			case x = <-d.tcompCmdC:
			case <-d.tcompTriggerC:
			case ch := <-d.tcompPauseC:
				d.pauseCompaction(ch)
				continue
			case _, _ = <-d.closeC:
				return
			}
		}
		if x != nil {
			switch cmd := x.(type) {
			case cIdle:
				ackQ = append(ackQ, x)
			case cRange:
				d.tableRangeCompaction(cmd.level, cmd.min, cmd.max)
				x.ack(nil)
			}
			x = nil
		}
		d.tableAutoCompaction()
	}
}
