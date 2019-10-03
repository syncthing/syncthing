// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	dbMaxOpenFiles = 100
	dbFlushBatch   = 4 << MiB

	// A large database is > 200 MiB. It's a mostly arbitrary value, but
	// it's also the case that each file is 2 MiB by default and when we
	// have dbMaxOpenFiles of them we will need to start thrashing fd:s.
	// Switching to large database settings causes larger files to be used
	// when compacting, reducing the number.
	dbLargeThreshold = dbMaxOpenFiles * (2 << MiB)

	KiB = 10
	MiB = 20
)

type Tuning int

const (
	// N.b. these constants must match those in lib/config.Tuning!
	TuningAuto Tuning = iota
	TuningSmall
	TuningLarge
)

// Lowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual *leveldb.DB database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one Lowlevel for
// any given *leveldb.DB.
type Lowlevel struct {
	committed int64 // atomic, must come first
	*leveldb.DB
	location  string
	folderIdx *smallIndex
	deviceIdx *smallIndex
	closed    bool
	closeMut  *sync.RWMutex
	iterWG    sync.WaitGroup
}

// Open attempts to open the database at the given location, and runs
// recovery on it if opening fails. Worst case, if recovery is not possible,
// the database is erased and created from scratch.
func Open(location string, tuning Tuning) (*Lowlevel, error) {
	opts := optsFor(location, tuning)
	return open(location, opts)
}

// optsFor returns the database options to use when opening a database with
// the given location and tuning. Settings can be overridden by debug
// environment variables.
func optsFor(location string, tuning Tuning) *opt.Options {
	large := false
	switch tuning {
	case TuningLarge:
		large = true
	case TuningAuto:
		large = dbIsLarge(location)
	}

	var (
		// Set defaults used for small databases.
		defaultBlockCacheCapacity            = 0 // 0 means let leveldb use default
		defaultBlockSize                     = 0
		defaultCompactionTableSize           = 0
		defaultCompactionTableSizeMultiplier = 0
		defaultWriteBuffer                   = 16 << MiB                      // increased from leveldb default of 4 MiB
		defaultCompactionL0Trigger           = opt.DefaultCompactionL0Trigger // explicit because we use it as base for other stuff
	)

	if large {
		// Change the parameters for better throughput at the price of some
		// RAM and larger files. This results in larger batches of writes
		// and compaction at a lower frequency.
		l.Infoln("Using large-database tuning")

		defaultBlockCacheCapacity = 64 << MiB
		defaultBlockSize = 64 << KiB
		defaultCompactionTableSize = 16 << MiB
		defaultCompactionTableSizeMultiplier = 20 // 2.0 after division by ten
		defaultWriteBuffer = 64 << MiB
		defaultCompactionL0Trigger = 8 // number of l0 files
	}

	opts := &opt.Options{
		BlockCacheCapacity:            debugEnvValue("BlockCacheCapacity", defaultBlockCacheCapacity),
		BlockCacheEvictRemoved:        debugEnvValue("BlockCacheEvictRemoved", 0) != 0,
		BlockRestartInterval:          debugEnvValue("BlockRestartInterval", 0),
		BlockSize:                     debugEnvValue("BlockSize", defaultBlockSize),
		CompactionExpandLimitFactor:   debugEnvValue("CompactionExpandLimitFactor", 0),
		CompactionGPOverlapsFactor:    debugEnvValue("CompactionGPOverlapsFactor", 0),
		CompactionL0Trigger:           debugEnvValue("CompactionL0Trigger", defaultCompactionL0Trigger),
		CompactionSourceLimitFactor:   debugEnvValue("CompactionSourceLimitFactor", 0),
		CompactionTableSize:           debugEnvValue("CompactionTableSize", defaultCompactionTableSize),
		CompactionTableSizeMultiplier: float64(debugEnvValue("CompactionTableSizeMultiplier", defaultCompactionTableSizeMultiplier)) / 10.0,
		CompactionTotalSize:           debugEnvValue("CompactionTotalSize", 0),
		CompactionTotalSizeMultiplier: float64(debugEnvValue("CompactionTotalSizeMultiplier", 0)) / 10.0,
		DisableBufferPool:             debugEnvValue("DisableBufferPool", 0) != 0,
		DisableBlockCache:             debugEnvValue("DisableBlockCache", 0) != 0,
		DisableCompactionBackoff:      debugEnvValue("DisableCompactionBackoff", 0) != 0,
		DisableLargeBatchTransaction:  debugEnvValue("DisableLargeBatchTransaction", 0) != 0,
		NoSync:                        debugEnvValue("NoSync", 0) != 0,
		NoWriteMerge:                  debugEnvValue("NoWriteMerge", 0) != 0,
		OpenFilesCacheCapacity:        debugEnvValue("OpenFilesCacheCapacity", dbMaxOpenFiles),
		WriteBuffer:                   debugEnvValue("WriteBuffer", defaultWriteBuffer),
		// The write slowdown and pause can be overridden, but even if they
		// are not and the compaction trigger is overridden we need to
		// adjust so that we don't pause writes for L0 compaction before we
		// even *start* L0 compaction...
		WriteL0SlowdownTrigger: debugEnvValue("WriteL0SlowdownTrigger", 2*debugEnvValue("CompactionL0Trigger", defaultCompactionL0Trigger)),
		WriteL0PauseTrigger:    debugEnvValue("WriteL0SlowdownTrigger", 3*debugEnvValue("CompactionL0Trigger", defaultCompactionL0Trigger)),
	}

	return opts
}

// OpenRO attempts to open the database at the given location, read only.
func OpenRO(location string) (*Lowlevel, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: dbMaxOpenFiles,
		ReadOnly:               true,
	}
	return open(location, opts)
}

func open(location string, opts *opt.Options) (*Lowlevel, error) {
	db, err := leveldb.OpenFile(location, opts)
	if leveldbIsCorrupted(err) {
		db, err = leveldb.RecoverFile(location, opts)
	}
	if leveldbIsCorrupted(err) {
		// The database is corrupted, and we've tried to recover it but it
		// didn't work. At this point there isn't much to do beyond dropping
		// the database and reindexing...
		l.Infoln("Database corruption detected, unable to recover. Reinitializing...")
		if err := os.RemoveAll(location); err != nil {
			return nil, errorSuggestion{err, "failed to delete corrupted database"}
		}
		db, err = leveldb.OpenFile(location, opts)
	}
	if err != nil {
		return nil, errorSuggestion{err, "is another instance of Syncthing running?"}
	}

	if debugEnvValue("CompactEverything", 0) != 0 {
		if err := db.CompactRange(util.Range{}); err != nil {
			l.Warnln("Compacting database:", err)
		}
	}

	return NewLowlevel(db, location), nil
}

// OpenMemory returns a new Lowlevel referencing an in-memory database.
func OpenMemory() *Lowlevel {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return NewLowlevel(db, "<memory>")
}

// ListFolders returns the list of folders currently in the database
func (db *Lowlevel) ListFolders() []string {
	return db.folderIdx.Values()
}

// Committed returns the number of items committed to the database since startup
func (db *Lowlevel) Committed() int64 {
	return atomic.LoadInt64(&db.committed)
}

func (db *Lowlevel) Put(key, val []byte, wo *opt.WriteOptions) error {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return leveldb.ErrClosed
	}
	atomic.AddInt64(&db.committed, 1)
	return db.DB.Put(key, val, wo)
}

func (db *Lowlevel) Write(batch *leveldb.Batch, wo *opt.WriteOptions) error {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return leveldb.ErrClosed
	}
	return db.DB.Write(batch, wo)
}

func (db *Lowlevel) Delete(key []byte, wo *opt.WriteOptions) error {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return leveldb.ErrClosed
	}
	atomic.AddInt64(&db.committed, 1)
	return db.DB.Delete(key, wo)
}

func (db *Lowlevel) NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	return db.newIterator(func() iterator.Iterator { return db.DB.NewIterator(slice, ro) })
}

// newIterator returns an iterator created with the given constructor only if db
// is not yet closed. If it is closed, a closedIter is returned instead.
func (db *Lowlevel) newIterator(constr func() iterator.Iterator) iterator.Iterator {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return &closedIter{}
	}
	db.iterWG.Add(1)
	return &iter{
		Iterator: constr(),
		db:       db,
	}
}

func (db *Lowlevel) GetSnapshot() snapshot {
	s, err := db.DB.GetSnapshot()
	if err != nil {
		if err == leveldb.ErrClosed {
			return &closedSnap{}
		}
		panic(err)
	}
	return &snap{
		Snapshot: s,
		db:       db,
	}
}

func (db *Lowlevel) Close() {
	db.closeMut.Lock()
	if db.closed {
		db.closeMut.Unlock()
		return
	}
	db.closed = true
	db.closeMut.Unlock()
	db.iterWG.Wait()
	db.DB.Close()
}

// dbIsLarge returns whether the estimated size of the database at location
// is large enough to warrant optimization for large databases.
func dbIsLarge(location string) bool {
	if ^uint(0)>>63 == 0 {
		// We're compiled for a 32 bit architecture. We've seen trouble with
		// large settings there.
		// (https://forum.syncthing.net/t/many-small-ldb-files-with-database-tuning/13842)
		return false
	}

	dir, err := os.Open(location)
	if err != nil {
		return false
	}

	fis, err := dir.Readdir(-1)
	if err != nil {
		return false
	}

	var size int64
	for _, fi := range fis {
		if fi.Name() == "LOG" {
			// don't count the size
			continue
		}
		size += fi.Size()
	}

	return size > dbLargeThreshold
}

// NewLowlevel wraps the given *leveldb.DB into a *lowlevel
func NewLowlevel(db *leveldb.DB, location string) *Lowlevel {
	return &Lowlevel{
		DB:        db,
		location:  location,
		folderIdx: newSmallIndex(db, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(db, []byte{KeyTypeDeviceIdx}),
		closeMut:  &sync.RWMutex{},
		iterWG:    sync.WaitGroup{},
	}
}

// A "better" version of leveldb's errors.IsCorrupted.
func leveldbIsCorrupted(err error) bool {
	switch {
	case err == nil:
		return false

	case errors.IsCorrupted(err):
		return true

	case strings.Contains(err.Error(), "corrupted"):
		return true
	}

	return false
}

type batch struct {
	*leveldb.Batch
	db *Lowlevel
}

func (db *Lowlevel) newBatch() *batch {
	return &batch{
		Batch: new(leveldb.Batch),
		db:    db,
	}
}

// checkFlush flushes and resets the batch if its size exceeds dbFlushBatch.
func (b *batch) checkFlush() {
	if len(b.Dump()) > dbFlushBatch {
		b.flush()
		b.Reset()
	}
}

func (b *batch) flush() {
	if err := b.db.Write(b.Batch, nil); err != nil && err != leveldb.ErrClosed {
		panic(err)
	}
}

type closedIter struct{}

func (it *closedIter) Release()                           {}
func (it *closedIter) Key() []byte                        { return nil }
func (it *closedIter) Value() []byte                      { return nil }
func (it *closedIter) Next() bool                         { return false }
func (it *closedIter) Prev() bool                         { return false }
func (it *closedIter) First() bool                        { return false }
func (it *closedIter) Last() bool                         { return false }
func (it *closedIter) Seek(key []byte) bool               { return false }
func (it *closedIter) Valid() bool                        { return false }
func (it *closedIter) Error() error                       { return leveldb.ErrClosed }
func (it *closedIter) SetReleaser(releaser util.Releaser) {}

type snapshot interface {
	Get([]byte, *opt.ReadOptions) ([]byte, error)
	Has([]byte, *opt.ReadOptions) (bool, error)
	NewIterator(*util.Range, *opt.ReadOptions) iterator.Iterator
	Release()
}

type closedSnap struct{}

func (s *closedSnap) Get([]byte, *opt.ReadOptions) ([]byte, error) { return nil, leveldb.ErrClosed }
func (s *closedSnap) Has([]byte, *opt.ReadOptions) (bool, error)   { return false, leveldb.ErrClosed }
func (s *closedSnap) NewIterator(*util.Range, *opt.ReadOptions) iterator.Iterator {
	return &closedIter{}
}
func (s *closedSnap) Release() {}

type snap struct {
	*leveldb.Snapshot
	db *Lowlevel
}

func (s *snap) NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	return s.db.newIterator(func() iterator.Iterator { return s.Snapshot.NewIterator(slice, ro) })
}

// iter implements iterator.Iterator which allows tracking active iterators
// and aborts if the underlying database is being closed.
type iter struct {
	iterator.Iterator
	db *Lowlevel
}

func (it *iter) Release() {
	it.db.iterWG.Done()
	it.Iterator.Release()
}

func (it *iter) Next() bool {
	return it.execIfNotClosed(it.Iterator.Next)
}
func (it *iter) Prev() bool {
	return it.execIfNotClosed(it.Iterator.Prev)
}
func (it *iter) First() bool {
	return it.execIfNotClosed(it.Iterator.First)
}
func (it *iter) Last() bool {
	return it.execIfNotClosed(it.Iterator.Last)
}
func (it *iter) Seek(key []byte) bool {
	return it.execIfNotClosed(func() bool {
		return it.Iterator.Seek(key)
	})
}

func (it *iter) execIfNotClosed(fn func() bool) bool {
	it.db.closeMut.RLock()
	defer it.db.closeMut.RUnlock()
	if it.db.closed {
		return false
	}
	return fn()
}

func debugEnvValue(key string, def int) int {
	v, err := strconv.ParseInt(os.Getenv("STDEBUG_"+key), 10, 63)
	if err != nil {
		return def
	}
	return int(v)
}
