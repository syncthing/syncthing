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
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	dbMaxOpenFiles = 100
	dbWriteBuffer  = 4 << 20
)

var (
	// Flush batches to disk when they contain this many records.
	batchFlushSize = debugEnvValue("BatchFlushSize", 64)
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
}

// Open attempts to open the database at the given location, and runs
// recovery on it if opening fails. Worst case, if recovery is not possible,
// the database is erased and created from scratch.
func Open(location string) (*Lowlevel, error) {
	opts := &opt.Options{
		BlockCacheCapacity:            debugEnvValue("BlockCacheCapacity", 0),
		BlockRestartInterval:          debugEnvValue("BlockRestartInterval", 0),
		BlockSize:                     debugEnvValue("BlockSize", 0),
		CompactionExpandLimitFactor:   debugEnvValue("CompactionExpandLimitFactor", 0),
		CompactionGPOverlapsFactor:    debugEnvValue("CompactionGPOverlapsFactor", 0),
		CompactionL0Trigger:           debugEnvValue("CompactionL0Trigger", 0),
		CompactionSourceLimitFactor:   debugEnvValue("CompactionSourceLimitFactor", 0),
		CompactionTableSize:           debugEnvValue("CompactionTableSize", 0),
		CompactionTableSizeMultiplier: float64(debugEnvValue("CompactionTableSizeMultiplier", 0)) / 10.0,
		CompactionTotalSize:           debugEnvValue("CompactionTotalSize", 0),
		CompactionTotalSizeMultiplier: float64(debugEnvValue("CompactionTotalSizeMultiplier", 0)) / 10.0,
		DisableBufferPool:             debugEnvValue("DisableBufferPool", 0) != 0,
		DisableBlockCache:             debugEnvValue("DisableBlockCache", 0) != 0,
		DisableCompactionBackoff:      debugEnvValue("DisableCompactionBackoff", 0) != 0,
		DisableLargeBatchTransaction:  debugEnvValue("DisableLargeBatchTransaction", 0) != 0,
		NoSync:                        debugEnvValue("NoSync", 0) != 0,
		NoWriteMerge:                  debugEnvValue("NoWriteMerge", 0) != 0,
		OpenFilesCacheCapacity:        debugEnvValue("OpenFilesCacheCapacity", dbMaxOpenFiles),
		WriteBuffer:                   debugEnvValue("WriteBuffer", dbWriteBuffer),
		// The write slowdown and pause can be overridden, but even if they
		// are not and the compaction trigger is overridden we need to
		// adjust so that we don't pause writes for L0 compaction before we
		// even *start* L0 compaction...
		WriteL0SlowdownTrigger: debugEnvValue("WriteL0SlowdownTrigger", 2*debugEnvValue("CompactionL0Trigger", opt.DefaultCompactionL0Trigger)),
		WriteL0PauseTrigger:    debugEnvValue("WriteL0SlowdownTrigger", 3*debugEnvValue("CompactionL0Trigger", opt.DefaultCompactionL0Trigger)),
	}
	return open(location, opts)
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

// Location returns the filesystem path where the database is stored
func (db *Lowlevel) Location() string {
	return db.location
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
	atomic.AddInt64(&db.committed, 1)
	return db.DB.Put(key, val, wo)
}

func (db *Lowlevel) Delete(key []byte, wo *opt.WriteOptions) error {
	atomic.AddInt64(&db.committed, 1)
	return db.DB.Delete(key, wo)
}

// NewLowlevel wraps the given *leveldb.DB into a *lowlevel
func NewLowlevel(db *leveldb.DB, location string) *Lowlevel {
	return &Lowlevel{
		DB:        db,
		location:  location,
		folderIdx: newSmallIndex(db, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(db, []byte{KeyTypeDeviceIdx}),
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

func debugEnvValue(key string, def int) int {
	v, err := strconv.ParseInt(os.Getenv("STDEBUG_"+key), 10, 63)
	if err != nil {
		return def
	}
	return int(v)
}
