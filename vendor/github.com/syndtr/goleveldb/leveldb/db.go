// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"container/list"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/table"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// DB is a LevelDB database.
type DB struct {
	// Need 64-bit alignment.
	seq uint64

	// Session.
	s *session

	// MemDB.
	memMu           sync.RWMutex
	memPool         chan *memdb.DB
	mem, frozenMem  *memDB
	journal         *journal.Writer
	journalWriter   storage.Writer
	journalFd       storage.FileDesc
	frozenJournalFd storage.FileDesc
	frozenSeq       uint64

	// Snapshot.
	snapsMu   sync.Mutex
	snapsList *list.List

	// Stats.
	aliveSnaps, aliveIters int32

	// Write.
	batchPool    sync.Pool
	writeMergeC  chan writeMerge
	writeMergedC chan bool
	writeLockC   chan struct{}
	writeAckC    chan error
	writeDelay   time.Duration
	writeDelayN  int
	tr           *Transaction

	// Compaction.
	compCommitLk     sync.Mutex
	tcompCmdC        chan cCmd
	tcompPauseC      chan chan<- struct{}
	mcompCmdC        chan cCmd
	compErrC         chan error
	compPerErrC      chan error
	compErrSetC      chan error
	compWriteLocking bool
	compStats        cStats
	memdbMaxLevel    int // For testing.

	// Close.
	closeW sync.WaitGroup
	closeC chan struct{}
	closed uint32
	closer io.Closer
}

func openDB(s *session) (*DB, error) {
	s.log("db@open opening")
	start := time.Now()
	db := &DB{
		s: s,
		// Initial sequence
		seq: s.stSeqNum,
		// MemDB
		memPool: make(chan *memdb.DB, 1),
		// Snapshot
		snapsList: list.New(),
		// Write
		batchPool:    sync.Pool{New: newBatch},
		writeMergeC:  make(chan writeMerge),
		writeMergedC: make(chan bool),
		writeLockC:   make(chan struct{}, 1),
		writeAckC:    make(chan error),
		// Compaction
		tcompCmdC:   make(chan cCmd),
		tcompPauseC: make(chan chan<- struct{}),
		mcompCmdC:   make(chan cCmd),
		compErrC:    make(chan error),
		compPerErrC: make(chan error),
		compErrSetC: make(chan error),
		// Close
		closeC: make(chan struct{}),
	}

	// Read-only mode.
	readOnly := s.o.GetReadOnly()

	if readOnly {
		// Recover journals (read-only mode).
		if err := db.recoverJournalRO(); err != nil {
			return nil, err
		}
	} else {
		// Recover journals.
		if err := db.recoverJournal(); err != nil {
			return nil, err
		}

		// Remove any obsolete files.
		if err := db.checkAndCleanFiles(); err != nil {
			// Close journal.
			if db.journal != nil {
				db.journal.Close()
				db.journalWriter.Close()
			}
			return nil, err
		}

	}

	// Doesn't need to be included in the wait group.
	go db.compactionError()
	go db.mpoolDrain()

	if readOnly {
		db.SetReadOnly()
	} else {
		db.closeW.Add(2)
		go db.tCompaction()
		go db.mCompaction()
		// go db.jWriter()
	}

	s.logf("db@open done T·%v", time.Since(start))

	runtime.SetFinalizer(db, (*DB).Close)
	return db, nil
}

// Open opens or creates a DB for the given storage.
// The DB will be created if not exist, unless ErrorIfMissing is true.
// Also, if ErrorIfExist is true and the DB exist Open will returns
// os.ErrExist error.
//
// Open will return an error with type of ErrCorrupted if corruption
// detected in the DB. Use errors.IsCorrupted to test whether an error is
// due to corruption. Corrupted DB can be recovered with Recover function.
//
// The returned DB instance is safe for concurrent use.
// The DB must be closed after use, by calling Close method.
func Open(stor storage.Storage, o *opt.Options) (db *DB, err error) {
	s, err := newSession(stor, o)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			s.close()
			s.release()
		}
	}()

	err = s.recover()
	if err != nil {
		if !os.IsNotExist(err) || s.o.GetErrorIfMissing() {
			return
		}
		err = s.create()
		if err != nil {
			return
		}
	} else if s.o.GetErrorIfExist() {
		err = os.ErrExist
		return
	}

	return openDB(s)
}

// OpenFile opens or creates a DB for the given path.
// The DB will be created if not exist, unless ErrorIfMissing is true.
// Also, if ErrorIfExist is true and the DB exist OpenFile will returns
// os.ErrExist error.
//
// OpenFile uses standard file-system backed storage implementation as
// described in the leveldb/storage package.
//
// OpenFile will return an error with type of ErrCorrupted if corruption
// detected in the DB. Use errors.IsCorrupted to test whether an error is
// due to corruption. Corrupted DB can be recovered with Recover function.
//
// The returned DB instance is safe for concurrent use.
// The DB must be closed after use, by calling Close method.
func OpenFile(path string, o *opt.Options) (db *DB, err error) {
	stor, err := storage.OpenFile(path, o.GetReadOnly())
	if err != nil {
		return
	}
	db, err = Open(stor, o)
	if err != nil {
		stor.Close()
	} else {
		db.closer = stor
	}
	return
}

// Recover recovers and opens a DB with missing or corrupted manifest files
// for the given storage. It will ignore any manifest files, valid or not.
// The DB must already exist or it will returns an error.
// Also, Recover will ignore ErrorIfMissing and ErrorIfExist options.
//
// The returned DB instance is safe for concurrent use.
// The DB must be closed after use, by calling Close method.
func Recover(stor storage.Storage, o *opt.Options) (db *DB, err error) {
	s, err := newSession(stor, o)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			s.close()
			s.release()
		}
	}()

	err = recoverTable(s, o)
	if err != nil {
		return
	}
	return openDB(s)
}

// RecoverFile recovers and opens a DB with missing or corrupted manifest files
// for the given path. It will ignore any manifest files, valid or not.
// The DB must already exist or it will returns an error.
// Also, Recover will ignore ErrorIfMissing and ErrorIfExist options.
//
// RecoverFile uses standard file-system backed storage implementation as described
// in the leveldb/storage package.
//
// The returned DB instance is safe for concurrent use.
// The DB must be closed after use, by calling Close method.
func RecoverFile(path string, o *opt.Options) (db *DB, err error) {
	stor, err := storage.OpenFile(path, false)
	if err != nil {
		return
	}
	db, err = Recover(stor, o)
	if err != nil {
		stor.Close()
	} else {
		db.closer = stor
	}
	return
}

func recoverTable(s *session, o *opt.Options) error {
	o = dupOptions(o)
	// Mask StrictReader, lets StrictRecovery doing its job.
	o.Strict &= ^opt.StrictReader

	// Get all tables and sort it by file number.
	fds, err := s.stor.List(storage.TypeTable)
	if err != nil {
		return err
	}
	sortFds(fds)

	var (
		maxSeq                                                            uint64
		recoveredKey, goodKey, corruptedKey, corruptedBlock, droppedTable int

		// We will drop corrupted table.
		strict = o.GetStrict(opt.StrictRecovery)
		noSync = o.GetNoSync()

		rec   = &sessionRecord{}
		bpool = util.NewBufferPool(o.GetBlockSize() + 5)
	)
	buildTable := func(iter iterator.Iterator) (tmpFd storage.FileDesc, size int64, err error) {
		tmpFd = s.newTemp()
		writer, err := s.stor.Create(tmpFd)
		if err != nil {
			return
		}
		defer func() {
			writer.Close()
			if err != nil {
				s.stor.Remove(tmpFd)
				tmpFd = storage.FileDesc{}
			}
		}()

		// Copy entries.
		tw := table.NewWriter(writer, o)
		for iter.Next() {
			key := iter.Key()
			if validInternalKey(key) {
				err = tw.Append(key, iter.Value())
				if err != nil {
					return
				}
			}
		}
		err = iter.Error()
		if err != nil {
			return
		}
		err = tw.Close()
		if err != nil {
			return
		}
		if !noSync {
			err = writer.Sync()
			if err != nil {
				return
			}
		}
		size = int64(tw.BytesLen())
		return
	}
	recoverTable := func(fd storage.FileDesc) error {
		s.logf("table@recovery recovering @%d", fd.Num)
		reader, err := s.stor.Open(fd)
		if err != nil {
			return err
		}
		var closed bool
		defer func() {
			if !closed {
				reader.Close()
			}
		}()

		// Get file size.
		size, err := reader.Seek(0, 2)
		if err != nil {
			return err
		}

		var (
			tSeq                                     uint64
			tgoodKey, tcorruptedKey, tcorruptedBlock int
			imin, imax                               []byte
		)
		tr, err := table.NewReader(reader, size, fd, nil, bpool, o)
		if err != nil {
			return err
		}
		iter := tr.NewIterator(nil, nil)
		if itererr, ok := iter.(iterator.ErrorCallbackSetter); ok {
			itererr.SetErrorCallback(func(err error) {
				if errors.IsCorrupted(err) {
					s.logf("table@recovery block corruption @%d %q", fd.Num, err)
					tcorruptedBlock++
				}
			})
		}

		// Scan the table.
		for iter.Next() {
			key := iter.Key()
			_, seq, _, kerr := parseInternalKey(key)
			if kerr != nil {
				tcorruptedKey++
				continue
			}
			tgoodKey++
			if seq > tSeq {
				tSeq = seq
			}
			if imin == nil {
				imin = append([]byte{}, key...)
			}
			imax = append(imax[:0], key...)
		}
		if err := iter.Error(); err != nil {
			iter.Release()
			return err
		}
		iter.Release()

		goodKey += tgoodKey
		corruptedKey += tcorruptedKey
		corruptedBlock += tcorruptedBlock

		if strict && (tcorruptedKey > 0 || tcorruptedBlock > 0) {
			droppedTable++
			s.logf("table@recovery dropped @%d Gk·%d Ck·%d Cb·%d S·%d Q·%d", fd.Num, tgoodKey, tcorruptedKey, tcorruptedBlock, size, tSeq)
			return nil
		}

		if tgoodKey > 0 {
			if tcorruptedKey > 0 || tcorruptedBlock > 0 {
				// Rebuild the table.
				s.logf("table@recovery rebuilding @%d", fd.Num)
				iter := tr.NewIterator(nil, nil)
				tmpFd, newSize, err := buildTable(iter)
				iter.Release()
				if err != nil {
					return err
				}
				closed = true
				reader.Close()
				if err := s.stor.Rename(tmpFd, fd); err != nil {
					return err
				}
				size = newSize
			}
			if tSeq > maxSeq {
				maxSeq = tSeq
			}
			recoveredKey += tgoodKey
			// Add table to level 0.
			rec.addTable(0, fd.Num, size, imin, imax)
			s.logf("table@recovery recovered @%d Gk·%d Ck·%d Cb·%d S·%d Q·%d", fd.Num, tgoodKey, tcorruptedKey, tcorruptedBlock, size, tSeq)
		} else {
			droppedTable++
			s.logf("table@recovery unrecoverable @%d Ck·%d Cb·%d S·%d", fd.Num, tcorruptedKey, tcorruptedBlock, size)
		}

		return nil
	}

	// Recover all tables.
	if len(fds) > 0 {
		s.logf("table@recovery F·%d", len(fds))

		// Mark file number as used.
		s.markFileNum(fds[len(fds)-1].Num)

		for _, fd := range fds {
			if err := recoverTable(fd); err != nil {
				return err
			}
		}

		s.logf("table@recovery recovered F·%d N·%d Gk·%d Ck·%d Q·%d", len(fds), recoveredKey, goodKey, corruptedKey, maxSeq)
	}

	// Set sequence number.
	rec.setSeqNum(maxSeq)

	// Create new manifest.
	if err := s.create(); err != nil {
		return err
	}

	// Commit.
	return s.commit(rec)
}

func (db *DB) recoverJournal() error {
	// Get all journals and sort it by file number.
	rawFds, err := db.s.stor.List(storage.TypeJournal)
	if err != nil {
		return err
	}
	sortFds(rawFds)

	// Journals that will be recovered.
	var fds []storage.FileDesc
	for _, fd := range rawFds {
		if fd.Num >= db.s.stJournalNum || fd.Num == db.s.stPrevJournalNum {
			fds = append(fds, fd)
		}
	}

	var (
		ofd storage.FileDesc // Obsolete file.
		rec = &sessionRecord{}
	)

	// Recover journals.
	if len(fds) > 0 {
		db.logf("journal@recovery F·%d", len(fds))

		// Mark file number as used.
		db.s.markFileNum(fds[len(fds)-1].Num)

		var (
			// Options.
			strict      = db.s.o.GetStrict(opt.StrictJournal)
			checksum    = db.s.o.GetStrict(opt.StrictJournalChecksum)
			writeBuffer = db.s.o.GetWriteBuffer()

			jr       *journal.Reader
			mdb      = memdb.New(db.s.icmp, writeBuffer)
			buf      = &util.Buffer{}
			batchSeq uint64
			batchLen int
		)

		for _, fd := range fds {
			db.logf("journal@recovery recovering @%d", fd.Num)

			fr, err := db.s.stor.Open(fd)
			if err != nil {
				return err
			}

			// Create or reset journal reader instance.
			if jr == nil {
				jr = journal.NewReader(fr, dropper{db.s, fd}, strict, checksum)
			} else {
				jr.Reset(fr, dropper{db.s, fd}, strict, checksum)
			}

			// Flush memdb and remove obsolete journal file.
			if !ofd.Zero() {
				if mdb.Len() > 0 {
					if _, err := db.s.flushMemdb(rec, mdb, 0); err != nil {
						fr.Close()
						return err
					}
				}

				rec.setJournalNum(fd.Num)
				rec.setSeqNum(db.seq)
				if err := db.s.commit(rec); err != nil {
					fr.Close()
					return err
				}
				rec.resetAddedTables()

				db.s.stor.Remove(ofd)
				ofd = storage.FileDesc{}
			}

			// Replay journal to memdb.
			mdb.Reset()
			for {
				r, err := jr.Next()
				if err != nil {
					if err == io.EOF {
						break
					}

					fr.Close()
					return errors.SetFd(err, fd)
				}

				buf.Reset()
				if _, err := buf.ReadFrom(r); err != nil {
					if err == io.ErrUnexpectedEOF {
						// This is error returned due to corruption, with strict == false.
						continue
					}

					fr.Close()
					return errors.SetFd(err, fd)
				}
				batchSeq, batchLen, err = decodeBatchToMem(buf.Bytes(), db.seq, mdb)
				if err != nil {
					if !strict && errors.IsCorrupted(err) {
						db.s.logf("journal error: %v (skipped)", err)
						// We won't apply sequence number as it might be corrupted.
						continue
					}

					fr.Close()
					return errors.SetFd(err, fd)
				}

				// Save sequence number.
				db.seq = batchSeq + uint64(batchLen)

				// Flush it if large enough.
				if mdb.Size() >= writeBuffer {
					if _, err := db.s.flushMemdb(rec, mdb, 0); err != nil {
						fr.Close()
						return err
					}

					mdb.Reset()
				}
			}

			fr.Close()
			ofd = fd
		}

		// Flush the last memdb.
		if mdb.Len() > 0 {
			if _, err := db.s.flushMemdb(rec, mdb, 0); err != nil {
				return err
			}
		}
	}

	// Create a new journal.
	if _, err := db.newMem(0); err != nil {
		return err
	}

	// Commit.
	rec.setJournalNum(db.journalFd.Num)
	rec.setSeqNum(db.seq)
	if err := db.s.commit(rec); err != nil {
		// Close journal on error.
		if db.journal != nil {
			db.journal.Close()
			db.journalWriter.Close()
		}
		return err
	}

	// Remove the last obsolete journal file.
	if !ofd.Zero() {
		db.s.stor.Remove(ofd)
	}

	return nil
}

func (db *DB) recoverJournalRO() error {
	// Get all journals and sort it by file number.
	rawFds, err := db.s.stor.List(storage.TypeJournal)
	if err != nil {
		return err
	}
	sortFds(rawFds)

	// Journals that will be recovered.
	var fds []storage.FileDesc
	for _, fd := range rawFds {
		if fd.Num >= db.s.stJournalNum || fd.Num == db.s.stPrevJournalNum {
			fds = append(fds, fd)
		}
	}

	var (
		// Options.
		strict      = db.s.o.GetStrict(opt.StrictJournal)
		checksum    = db.s.o.GetStrict(opt.StrictJournalChecksum)
		writeBuffer = db.s.o.GetWriteBuffer()

		mdb = memdb.New(db.s.icmp, writeBuffer)
	)

	// Recover journals.
	if len(fds) > 0 {
		db.logf("journal@recovery RO·Mode F·%d", len(fds))

		var (
			jr       *journal.Reader
			buf      = &util.Buffer{}
			batchSeq uint64
			batchLen int
		)

		for _, fd := range fds {
			db.logf("journal@recovery recovering @%d", fd.Num)

			fr, err := db.s.stor.Open(fd)
			if err != nil {
				return err
			}

			// Create or reset journal reader instance.
			if jr == nil {
				jr = journal.NewReader(fr, dropper{db.s, fd}, strict, checksum)
			} else {
				jr.Reset(fr, dropper{db.s, fd}, strict, checksum)
			}

			// Replay journal to memdb.
			for {
				r, err := jr.Next()
				if err != nil {
					if err == io.EOF {
						break
					}

					fr.Close()
					return errors.SetFd(err, fd)
				}

				buf.Reset()
				if _, err := buf.ReadFrom(r); err != nil {
					if err == io.ErrUnexpectedEOF {
						// This is error returned due to corruption, with strict == false.
						continue
					}

					fr.Close()
					return errors.SetFd(err, fd)
				}
				batchSeq, batchLen, err = decodeBatchToMem(buf.Bytes(), db.seq, mdb)
				if err != nil {
					if !strict && errors.IsCorrupted(err) {
						db.s.logf("journal error: %v (skipped)", err)
						// We won't apply sequence number as it might be corrupted.
						continue
					}

					fr.Close()
					return errors.SetFd(err, fd)
				}

				// Save sequence number.
				db.seq = batchSeq + uint64(batchLen)
			}

			fr.Close()
		}
	}

	// Set memDB.
	db.mem = &memDB{db: db, DB: mdb, ref: 1}

	return nil
}

func memGet(mdb *memdb.DB, ikey internalKey, icmp *iComparer) (ok bool, mv []byte, err error) {
	mk, mv, err := mdb.Find(ikey)
	if err == nil {
		ukey, _, kt, kerr := parseInternalKey(mk)
		if kerr != nil {
			// Shouldn't have had happen.
			panic(kerr)
		}
		if icmp.uCompare(ukey, ikey.ukey()) == 0 {
			if kt == keyTypeDel {
				return true, nil, ErrNotFound
			}
			return true, mv, nil

		}
	} else if err != ErrNotFound {
		return true, nil, err
	}
	return
}

func (db *DB) get(auxm *memdb.DB, auxt tFiles, key []byte, seq uint64, ro *opt.ReadOptions) (value []byte, err error) {
	ikey := makeInternalKey(nil, key, seq, keyTypeSeek)

	if auxm != nil {
		if ok, mv, me := memGet(auxm, ikey, db.s.icmp); ok {
			return append([]byte{}, mv...), me
		}
	}

	em, fm := db.getMems()
	for _, m := range [...]*memDB{em, fm} {
		if m == nil {
			continue
		}
		defer m.decref()

		if ok, mv, me := memGet(m.DB, ikey, db.s.icmp); ok {
			return append([]byte{}, mv...), me
		}
	}

	v := db.s.version()
	value, cSched, err := v.get(auxt, ikey, ro, false)
	v.release()
	if cSched {
		// Trigger table compaction.
		db.compTrigger(db.tcompCmdC)
	}
	return
}

func nilIfNotFound(err error) error {
	if err == ErrNotFound {
		return nil
	}
	return err
}

func (db *DB) has(auxm *memdb.DB, auxt tFiles, key []byte, seq uint64, ro *opt.ReadOptions) (ret bool, err error) {
	ikey := makeInternalKey(nil, key, seq, keyTypeSeek)

	if auxm != nil {
		if ok, _, me := memGet(auxm, ikey, db.s.icmp); ok {
			return me == nil, nilIfNotFound(me)
		}
	}

	em, fm := db.getMems()
	for _, m := range [...]*memDB{em, fm} {
		if m == nil {
			continue
		}
		defer m.decref()

		if ok, _, me := memGet(m.DB, ikey, db.s.icmp); ok {
			return me == nil, nilIfNotFound(me)
		}
	}

	v := db.s.version()
	_, cSched, err := v.get(auxt, ikey, ro, true)
	v.release()
	if cSched {
		// Trigger table compaction.
		db.compTrigger(db.tcompCmdC)
	}
	if err == nil {
		ret = true
	} else if err == ErrNotFound {
		err = nil
	}
	return
}

// Get gets the value for the given key. It returns ErrNotFound if the
// DB does not contains the key.
//
// The returned slice is its own copy, it is safe to modify the contents
// of the returned slice.
// It is safe to modify the contents of the argument after Get returns.
func (db *DB) Get(key []byte, ro *opt.ReadOptions) (value []byte, err error) {
	err = db.ok()
	if err != nil {
		return
	}

	se := db.acquireSnapshot()
	defer db.releaseSnapshot(se)
	return db.get(nil, nil, key, se.seq, ro)
}

// Has returns true if the DB does contains the given key.
//
// It is safe to modify the contents of the argument after Get returns.
func (db *DB) Has(key []byte, ro *opt.ReadOptions) (ret bool, err error) {
	err = db.ok()
	if err != nil {
		return
	}

	se := db.acquireSnapshot()
	defer db.releaseSnapshot(se)
	return db.has(nil, nil, key, se.seq, ro)
}

// NewIterator returns an iterator for the latest snapshot of the
// underlying DB.
// The returned iterator is not safe for concurrent use, but it is safe to use
// multiple iterators concurrently, with each in a dedicated goroutine.
// It is also safe to use an iterator concurrently with modifying its
// underlying DB. The resultant key/value pairs are guaranteed to be
// consistent.
//
// Slice allows slicing the iterator to only contains keys in the given
// range. A nil Range.Start is treated as a key before all keys in the
// DB. And a nil Range.Limit is treated as a key after all keys in
// the DB.
//
// The iterator must be released after use, by calling Release method.
//
// Also read Iterator documentation of the leveldb/iterator package.
func (db *DB) NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	if err := db.ok(); err != nil {
		return iterator.NewEmptyIterator(err)
	}

	se := db.acquireSnapshot()
	defer db.releaseSnapshot(se)
	// Iterator holds 'version' lock, 'version' is immutable so snapshot
	// can be released after iterator created.
	return db.newIterator(nil, nil, se.seq, slice, ro)
}

// GetSnapshot returns a latest snapshot of the underlying DB. A snapshot
// is a frozen snapshot of a DB state at a particular point in time. The
// content of snapshot are guaranteed to be consistent.
//
// The snapshot must be released after use, by calling Release method.
func (db *DB) GetSnapshot() (*Snapshot, error) {
	if err := db.ok(); err != nil {
		return nil, err
	}

	return db.newSnapshot(), nil
}

// GetProperty returns value of the given property name.
//
// Property names:
//	leveldb.num-files-at-level{n}
//		Returns the number of files at level 'n'.
//	leveldb.stats
//		Returns statistics of the underlying DB.
//	leveldb.sstables
//		Returns sstables list for each level.
//	leveldb.blockpool
//		Returns block pool stats.
//	leveldb.cachedblock
//		Returns size of cached block.
//	leveldb.openedtables
//		Returns number of opened tables.
//	leveldb.alivesnaps
//		Returns number of alive snapshots.
//	leveldb.aliveiters
//		Returns number of alive iterators.
func (db *DB) GetProperty(name string) (value string, err error) {
	err = db.ok()
	if err != nil {
		return
	}

	const prefix = "leveldb."
	if !strings.HasPrefix(name, prefix) {
		return "", ErrNotFound
	}
	p := name[len(prefix):]

	v := db.s.version()
	defer v.release()

	numFilesPrefix := "num-files-at-level"
	switch {
	case strings.HasPrefix(p, numFilesPrefix):
		var level uint
		var rest string
		n, _ := fmt.Sscanf(p[len(numFilesPrefix):], "%d%s", &level, &rest)
		if n != 1 {
			err = ErrNotFound
		} else {
			value = fmt.Sprint(v.tLen(int(level)))
		}
	case p == "stats":
		value = "Compactions\n" +
			" Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)\n" +
			"-------+------------+---------------+---------------+---------------+---------------\n"
		for level, tables := range v.levels {
			duration, read, write := db.compStats.getStat(level)
			if len(tables) == 0 && duration == 0 {
				continue
			}
			value += fmt.Sprintf(" %3d   | %10d | %13.5f | %13.5f | %13.5f | %13.5f\n",
				level, len(tables), float64(tables.size())/1048576.0, duration.Seconds(),
				float64(read)/1048576.0, float64(write)/1048576.0)
		}
	case p == "sstables":
		for level, tables := range v.levels {
			value += fmt.Sprintf("--- level %d ---\n", level)
			for _, t := range tables {
				value += fmt.Sprintf("%d:%d[%q .. %q]\n", t.fd.Num, t.size, t.imin, t.imax)
			}
		}
	case p == "blockpool":
		value = fmt.Sprintf("%v", db.s.tops.bpool)
	case p == "cachedblock":
		if db.s.tops.bcache != nil {
			value = fmt.Sprintf("%d", db.s.tops.bcache.Size())
		} else {
			value = "<nil>"
		}
	case p == "openedtables":
		value = fmt.Sprintf("%d", db.s.tops.cache.Size())
	case p == "alivesnaps":
		value = fmt.Sprintf("%d", atomic.LoadInt32(&db.aliveSnaps))
	case p == "aliveiters":
		value = fmt.Sprintf("%d", atomic.LoadInt32(&db.aliveIters))
	default:
		err = ErrNotFound
	}

	return
}

// SizeOf calculates approximate sizes of the given key ranges.
// The length of the returned sizes are equal with the length of the given
// ranges. The returned sizes measure storage space usage, so if the user
// data compresses by a factor of ten, the returned sizes will be one-tenth
// the size of the corresponding user data size.
// The results may not include the sizes of recently written data.
func (db *DB) SizeOf(ranges []util.Range) (Sizes, error) {
	if err := db.ok(); err != nil {
		return nil, err
	}

	v := db.s.version()
	defer v.release()

	sizes := make(Sizes, 0, len(ranges))
	for _, r := range ranges {
		imin := makeInternalKey(nil, r.Start, keyMaxSeq, keyTypeSeek)
		imax := makeInternalKey(nil, r.Limit, keyMaxSeq, keyTypeSeek)
		start, err := v.offsetOf(imin)
		if err != nil {
			return nil, err
		}
		limit, err := v.offsetOf(imax)
		if err != nil {
			return nil, err
		}
		var size int64
		if limit >= start {
			size = limit - start
		}
		sizes = append(sizes, size)
	}

	return sizes, nil
}

// Close closes the DB. This will also releases any outstanding snapshot,
// abort any in-flight compaction and discard open transaction.
//
// It is not safe to close a DB until all outstanding iterators are released.
// It is valid to call Close multiple times. Other methods should not be
// called after the DB has been closed.
func (db *DB) Close() error {
	if !db.setClosed() {
		return ErrClosed
	}

	start := time.Now()
	db.log("db@close closing")

	// Clear the finalizer.
	runtime.SetFinalizer(db, nil)

	// Get compaction error.
	var err error
	select {
	case err = <-db.compErrC:
		if err == ErrReadOnly {
			err = nil
		}
	default:
	}

	// Signal all goroutines.
	close(db.closeC)

	// Discard open transaction.
	if db.tr != nil {
		db.tr.Discard()
	}

	// Acquire writer lock.
	db.writeLockC <- struct{}{}

	// Wait for all gorotines to exit.
	db.closeW.Wait()

	// Closes journal.
	if db.journal != nil {
		db.journal.Close()
		db.journalWriter.Close()
		db.journal = nil
		db.journalWriter = nil
	}

	if db.writeDelayN > 0 {
		db.logf("db@write was delayed N·%d T·%v", db.writeDelayN, db.writeDelay)
	}

	// Close session.
	db.s.close()
	db.logf("db@close done T·%v", time.Since(start))
	db.s.release()

	if db.closer != nil {
		if err1 := db.closer.Close(); err == nil {
			err = err1
		}
		db.closer = nil
	}

	// Clear memdbs.
	db.clearMems()

	return err
}
