// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/opt"
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
	return &cMem{s: s, rec: &sessionRecord{numLevel: s.o.GetNumLevel()}}
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
		v := s.version()
		level = v.pickLevel(t.imin.ukey(), t.imax.ukey())
		v.release()
	}
	c.rec.addTableFile(level, t)

	s.logf("mem@flush created L%d@%d N·%d S·%s %q:%q", level, t.file.Num(), n, shortenb(int(t.size)), t.imin, t.imax)

	c.level = level
	return nil
}

func (c *cMem) reset() {
	c.rec = &sessionRecord{numLevel: c.s.o.GetNumLevel()}
}

func (c *cMem) commit(journal, seq uint64) error {
	c.rec.setJournalNum(journal)
	c.rec.setSeqNum(seq)

	// Commit changes.
	return c.s.commit(c.rec)
}

func (db *DB) compactionError() {
	var (
		err     error
		wlocked bool
	)
noerr:
	// No error.
	for {
		select {
		case err = <-db.compErrSetC:
			switch {
			case err == nil:
			case errors.IsCorrupted(err):
				goto hasperr
			default:
				goto haserr
			}
		case _, _ = <-db.closeC:
			return
		}
	}
haserr:
	// Transient error.
	for {
		select {
		case db.compErrC <- err:
		case err = <-db.compErrSetC:
			switch {
			case err == nil:
				goto noerr
			case errors.IsCorrupted(err):
				goto hasperr
			default:
			}
		case _, _ = <-db.closeC:
			return
		}
	}
hasperr:
	// Persistent error.
	for {
		select {
		case db.compErrC <- err:
		case db.compPerErrC <- err:
		case db.writeLockC <- struct{}{}:
			// Hold write lock, so that write won't pass-through.
			wlocked = true
		case _, _ = <-db.closeC:
			if wlocked {
				// We should release the lock or Close will hang.
				<-db.writeLockC
			}
			return
		}
	}
}

type compactionTransactCounter int

func (cnt *compactionTransactCounter) incr() {
	*cnt++
}

type compactionTransactInterface interface {
	run(cnt *compactionTransactCounter) error
	revert() error
}

func (db *DB) compactionTransact(name string, t compactionTransactInterface) {
	defer func() {
		if x := recover(); x != nil {
			if x == errCompactionTransactExiting {
				if err := t.revert(); err != nil {
					db.logf("%s revert error %q", name, err)
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
	var (
		backoff  = backoffMin
		backoffT = time.NewTimer(backoff)
		lastCnt  = compactionTransactCounter(0)

		disableBackoff = db.s.o.GetDisableCompactionBackoff()
	)
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
		err := t.run(&cnt)
		if err != nil {
			db.logf("%s error I·%d %q", name, cnt, err)
		}

		// Set compaction error status.
		select {
		case db.compErrSetC <- err:
		case perr := <-db.compPerErrC:
			if err != nil {
				db.logf("%s exiting (persistent error %q)", name, perr)
				db.compactionExitTransact()
			}
		case _, _ = <-db.closeC:
			db.logf("%s exiting", name)
			db.compactionExitTransact()
		}
		if err == nil {
			return
		}
		if errors.IsCorrupted(err) {
			db.logf("%s exiting (corruption detected)", name)
			db.compactionExitTransact()
		}

		if !disableBackoff {
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
}

type compactionTransactFunc struct {
	runFunc    func(cnt *compactionTransactCounter) error
	revertFunc func() error
}

func (t *compactionTransactFunc) run(cnt *compactionTransactCounter) error {
	return t.runFunc(cnt)
}

func (t *compactionTransactFunc) revert() error {
	if t.revertFunc != nil {
		return t.revertFunc()
	}
	return nil
}

func (db *DB) compactionTransactFunc(name string, run func(cnt *compactionTransactCounter) error, revert func() error) {
	db.compactionTransact(name, &compactionTransactFunc{run, revert})
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
	resumeC := make(chan struct{})
	select {
	case db.tcompPauseC <- (chan<- struct{})(resumeC):
	case <-db.compPerErrC:
		close(resumeC)
		resumeC = nil
	case _, _ = <-db.closeC:
		return
	}

	db.compactionTransactFunc("mem@flush", func(cnt *compactionTransactCounter) (err error) {
		stats.startTimer()
		defer stats.stopTimer()
		return c.flush(mem.mdb, -1)
	}, func() error {
		for _, r := range c.rec.addedTables {
			db.logf("mem@flush revert @%d", r.num)
			f := db.s.getTableFile(r.num)
			if err := f.Remove(); err != nil {
				return err
			}
		}
		return nil
	})

	db.compactionTransactFunc("mem@commit", func(cnt *compactionTransactCounter) (err error) {
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
	if resumeC != nil {
		select {
		case <-resumeC:
			close(resumeC)
		case _, _ = <-db.closeC:
			return
		}
	}

	// Trigger table compaction.
	db.compSendTrigger(db.tcompCmdC)
}

type tableCompactionBuilder struct {
	db           *DB
	s            *session
	c            *compaction
	rec          *sessionRecord
	stat0, stat1 *cStatsStaging

	snapHasLastUkey bool
	snapLastUkey    []byte
	snapLastSeq     uint64
	snapIter        int
	snapKerrCnt     int
	snapDropCnt     int

	kerrCnt int
	dropCnt int

	minSeq    uint64
	strict    bool
	tableSize int

	tw *tWriter
}

func (b *tableCompactionBuilder) appendKV(key, value []byte) error {
	// Create new table if not already.
	if b.tw == nil {
		// Check for pause event.
		if b.db != nil {
			select {
			case ch := <-b.db.tcompPauseC:
				b.db.pauseCompaction(ch)
			case _, _ = <-b.db.closeC:
				b.db.compactionExitTransact()
			default:
			}
		}

		// Create new table.
		var err error
		b.tw, err = b.s.tops.create()
		if err != nil {
			return err
		}
	}

	// Write key/value into table.
	return b.tw.append(key, value)
}

func (b *tableCompactionBuilder) needFlush() bool {
	return b.tw.tw.BytesLen() >= b.tableSize
}

func (b *tableCompactionBuilder) flush() error {
	t, err := b.tw.finish()
	if err != nil {
		return err
	}
	b.rec.addTableFile(b.c.level+1, t)
	b.stat1.write += t.size
	b.s.logf("table@build created L%d@%d N·%d S·%s %q:%q", b.c.level+1, t.file.Num(), b.tw.tw.EntriesLen(), shortenb(int(t.size)), t.imin, t.imax)
	b.tw = nil
	return nil
}

func (b *tableCompactionBuilder) cleanup() {
	if b.tw != nil {
		b.tw.drop()
		b.tw = nil
	}
}

func (b *tableCompactionBuilder) run(cnt *compactionTransactCounter) error {
	snapResumed := b.snapIter > 0
	hasLastUkey := b.snapHasLastUkey // The key might has zero length, so this is necessary.
	lastUkey := append([]byte{}, b.snapLastUkey...)
	lastSeq := b.snapLastSeq
	b.kerrCnt = b.snapKerrCnt
	b.dropCnt = b.snapDropCnt
	// Restore compaction state.
	b.c.restore()

	defer b.cleanup()

	b.stat1.startTimer()
	defer b.stat1.stopTimer()

	iter := b.c.newIterator()
	defer iter.Release()
	for i := 0; iter.Next(); i++ {
		// Incr transact counter.
		cnt.incr()

		// Skip until last state.
		if i < b.snapIter {
			continue
		}

		resumed := false
		if snapResumed {
			resumed = true
			snapResumed = false
		}

		ikey := iter.Key()
		ukey, seq, kt, kerr := parseIkey(ikey)

		if kerr == nil {
			shouldStop := !resumed && b.c.shouldStopBefore(ikey)

			if !hasLastUkey || b.s.icmp.uCompare(lastUkey, ukey) != 0 {
				// First occurrence of this user key.

				// Only rotate tables if ukey doesn't hop across.
				if b.tw != nil && (shouldStop || b.needFlush()) {
					if err := b.flush(); err != nil {
						return err
					}

					// Creates snapshot of the state.
					b.c.save()
					b.snapHasLastUkey = hasLastUkey
					b.snapLastUkey = append(b.snapLastUkey[:0], lastUkey...)
					b.snapLastSeq = lastSeq
					b.snapIter = i
					b.snapKerrCnt = b.kerrCnt
					b.snapDropCnt = b.dropCnt
				}

				hasLastUkey = true
				lastUkey = append(lastUkey[:0], ukey...)
				lastSeq = kMaxSeq
			}

			switch {
			case lastSeq <= b.minSeq:
				// Dropped because newer entry for same user key exist
				fallthrough // (A)
			case kt == ktDel && seq <= b.minSeq && b.c.baseLevelForKey(lastUkey):
				// For this user key:
				// (1) there is no data in higher levels
				// (2) data in lower levels will have larger seq numbers
				// (3) data in layers that are being compacted here and have
				//     smaller seq numbers will be dropped in the next
				//     few iterations of this loop (by rule (A) above).
				// Therefore this deletion marker is obsolete and can be dropped.
				lastSeq = seq
				b.dropCnt++
				continue
			default:
				lastSeq = seq
			}
		} else {
			if b.strict {
				return kerr
			}

			// Don't drop corrupted keys.
			hasLastUkey = false
			lastUkey = lastUkey[:0]
			lastSeq = kMaxSeq
			b.kerrCnt++
		}

		if err := b.appendKV(ikey, iter.Value()); err != nil {
			return err
		}
	}

	if err := iter.Error(); err != nil {
		return err
	}

	// Finish last table.
	if b.tw != nil && !b.tw.empty() {
		return b.flush()
	}
	return nil
}

func (b *tableCompactionBuilder) revert() error {
	for _, at := range b.rec.addedTables {
		b.s.logf("table@build revert @%d", at.num)
		f := b.s.getTableFile(at.num)
		if err := f.Remove(); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) tableCompaction(c *compaction, noTrivial bool) {
	defer c.release()

	rec := &sessionRecord{numLevel: db.s.o.GetNumLevel()}
	rec.addCompPtr(c.level, c.imax)

	if !noTrivial && c.trivial() {
		t := c.tables[0][0]
		db.logf("table@move L%d@%d -> L%d", c.level, t.file.Num(), c.level+1)
		rec.delTable(c.level, t.file.Num())
		rec.addTableFile(c.level+1, t)
		db.compactionTransactFunc("table@move", func(cnt *compactionTransactCounter) (err error) {
			return db.s.commit(rec)
		}, nil)
		return
	}

	var stats [2]cStatsStaging
	for i, tables := range c.tables {
		for _, t := range tables {
			stats[i].read += t.size
			// Insert deleted tables into record
			rec.delTable(c.level+i, t.file.Num())
		}
	}
	sourceSize := int(stats[0].read + stats[1].read)
	minSeq := db.minSeq()
	db.logf("table@compaction L%d·%d -> L%d·%d S·%s Q·%d", c.level, len(c.tables[0]), c.level+1, len(c.tables[1]), shortenb(sourceSize), minSeq)

	b := &tableCompactionBuilder{
		db:        db,
		s:         db.s,
		c:         c,
		rec:       rec,
		stat1:     &stats[1],
		minSeq:    minSeq,
		strict:    db.s.o.GetStrict(opt.StrictCompaction),
		tableSize: db.s.o.GetCompactionTableSize(c.level + 1),
	}
	db.compactionTransact("table@build", b)

	// Commit changes
	db.compactionTransactFunc("table@commit", func(cnt *compactionTransactCounter) (err error) {
		stats[1].startTimer()
		defer stats[1].stopTimer()
		return db.s.commit(rec)
	}, nil)

	resultSize := int(stats[1].write)
	db.logf("table@compaction committed F%s S%s Ke·%d D·%d T·%v", sint(len(rec.addedTables)-len(rec.deletedTables)), sshortenb(resultSize-sourceSize), b.kerrCnt, b.dropCnt, stats[1].duration)

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
		v := db.s.version()
		m := 1
		for i, t := range v.tables[1:] {
			if t.overlaps(db.s.icmp, umin, umax, false) {
				m = i + 1
			}
		}
		v.release()

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
	v := db.s.version()
	defer v.release()
	return v.needCompaction()
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
	if r.ackC != nil {
		defer func() {
			recover()
		}()
		r.ackC <- err
	}
}

type cRange struct {
	level    int
	min, max []byte
	ackC     chan<- error
}

func (r cRange) ack(err error) {
	if r.ackC != nil {
		defer func() {
			recover()
		}()
		r.ackC <- err
	}
}

// This will trigger auto compation and/or wait for all compaction to be done.
func (db *DB) compSendIdle(compC chan<- cCmd) (err error) {
	ch := make(chan error)
	defer close(ch)
	// Send cmd.
	select {
	case compC <- cIdle{ch}:
	case err = <-db.compErrC:
		return
	case _, _ = <-db.closeC:
		return ErrClosed
	}
	// Wait cmd.
	select {
	case err = <-ch:
	case err = <-db.compErrC:
	case _, _ = <-db.closeC:
		return ErrClosed
	}
	return err
}

// This will trigger auto compaction but will not wait for it.
func (db *DB) compSendTrigger(compC chan<- cCmd) {
	select {
	case compC <- cIdle{}:
	default:
	}
}

// Send range compaction request.
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
	case err = <-ch:
	case err = <-db.compErrC:
	case _, _ = <-db.closeC:
		return ErrClosed
	}
	return err
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
			switch x.(type) {
			case cIdle:
				db.memCompaction()
				x.ack(nil)
				x = nil
			default:
				panic("leveldb: unknown command")
			}
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
			default:
				panic("leveldb: unknown command")
			}
			x = nil
		}
		db.tableAutoCompaction()
	}
}
