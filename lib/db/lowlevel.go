// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"os"
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
	dbWriteBuffer  = 16 << 20
	dbFlushBatch   = dbWriteBuffer / 4 // Some leeway for any leveldb in-memory optimizations
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
}

// Open attempts to open the database at the given location, and runs
// recovery on it if opening fails. Worst case, if recovery is not possible,
// the database is erased and created from scratch.
func Open(location string) (*Lowlevel, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: dbMaxOpenFiles,
		WriteBuffer:            dbWriteBuffer,
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
	atomic.AddInt64(&db.committed, 1)
	return db.DB.Put(key, val, wo)
}

func (db *Lowlevel) Delete(key []byte, wo *opt.WriteOptions) error {
	atomic.AddInt64(&db.committed, 1)
	return db.DB.Delete(key, wo)
}

func (db *Lowlevel) NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return &closedIter{}
	}
	return db.DB.NewIterator(slice, ro)
}

func (db *Lowlevel) GetSnapshot() snapshot {
	snap, err := db.DB.GetSnapshot()
	if err != nil {
		if err == leveldb.ErrClosed {
			return &closedSnap{}
		}
		panic(err)
	}
	return snap
}

func (db *Lowlevel) Close() {
	db.closeMut.Lock()
	defer db.closeMut.Unlock()
	if db.closed {
		return
	}
	db.closed = true
	db.DB.Close()
}

// NewLowlevel wraps the given *leveldb.DB into a *lowlevel
func NewLowlevel(db *leveldb.DB, location string) *Lowlevel {
	return &Lowlevel{
		DB:        db,
		location:  location,
		folderIdx: newSmallIndex(db, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(db, []byte{KeyTypeDeviceIdx}),
		closeMut:  &sync.RWMutex{},
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
