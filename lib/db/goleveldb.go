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

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	goleveldbMaxOpenFiles = 100
	goleveldbWriteBuffer  = 16 << 20
	goleveldbFlushBatch   = goleveldbWriteBuffer / 4 // Some leeway for any leveldb in-memory optimizations
)

type goleveldb struct {
	*leveldb.DB
	closed   bool
	closeMut *sync.RWMutex
	iterWG   sync.WaitGroup
}

// NewInstanceFromGoleveldb attempts to open the database at the given location,
// and runs recovery on it if opening fails. Worst case, if recovery is not possible,
// the database is erased and created from scratch.
func NewInstanceFromGoleveldb(location string) (*Instance, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: goleveldbMaxOpenFiles,
		WriteBuffer:            goleveldbWriteBuffer,
	}
	db, err := openGoleveldb(location, opts)
	if err != nil {
		return nil, err
	}
	return NewInstance(db), nil
}

// NewROInstanceFromGoleveldb attempts to open the database at the given location, read only.
func NewROInstanceFromGoleveldb(location string) (*Instance, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: goleveldbMaxOpenFiles,
		ReadOnly:               true,
	}
	db, err := openGoleveldb(location, opts)
	if err != nil {
		return nil, err
	}
	return NewInstance(db), nil
}

func openGoleveldb(location string, opts *opt.Options) (*goleveldb, error) {
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
	return &goleveldb{
		DB:       db,
		closeMut: &sync.RWMutex{},
		iterWG:   sync.WaitGroup{},
	}, nil
}

// OpenMemory returns a new Instance referencing an in-memory database.
func OpenMemory() *Instance {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return NewInstance(&goleveldb{
		DB:       db,
		closeMut: &sync.RWMutex{},
		iterWG:   sync.WaitGroup{},
	})
}

func (db *goleveldb) Get(key []byte) ([]byte, error) {
	val, err := db.DB.Get(key, nil)
	return val, db.convertError(err)
}

func (db *goleveldb) Has(key []byte) (bool, error) {
	has, err := db.DB.Has(key, nil)
	return has, db.convertError(err)
}

func (db *goleveldb) Put(key, val []byte) error {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return leveldb.ErrClosed
	}
	return db.convertError(db.DB.Put(key, val, nil))
}

func (db *goleveldb) Delete(key []byte) error {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return leveldb.ErrClosed
	}
	return db.convertError(db.DB.Delete(key, nil))
}

func (db *goleveldb) NewIterator(slice *util.Range) (iterator.Iterator, error) {
	return db.newIterator(func() iterator.Iterator {
		return db.DB.NewIterator(slice, nil)
	})
}

// newIterator returns an iterator created with the given constructor only if db
// is not yet closed. If it is closed, a closedIter is returned instead.
func (db *goleveldb) newIterator(constr func() iterator.Iterator) (iterator.Iterator, error) {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return nil, leveldb.ErrClosed
	}
	db.iterWG.Add(1)
	return &iter{
		Iterator: constr(),
		db:       db,
	}, nil
}

func (db *goleveldb) GetSnapshot() (Snapshot, error) {
	db.closeMut.RLock()
	defer db.closeMut.RUnlock()
	if db.closed {
		return nil, leveldb.ErrClosed
	}
	s, err := db.DB.GetSnapshot()
	if err != nil {
		return nil, db.convertError(err)
	}
	return &snap{
		Snapshot: s,
		db:       db,
	}, nil
}

func (db *goleveldb) Close() {
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

func (db *goleveldb) convertError(err error) error {
	switch err {
	case leveldb.ErrClosed:
		return ErrClosed
	case leveldb.ErrNotFound:
		return ErrNotFound
	}
	return err
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

type goleveldbBatch struct {
	*leveldb.Batch
	db *goleveldb
}

func (db *goleveldb) NewBatch() Batch {
	return &goleveldbBatch{
		Batch: new(leveldb.Batch),
		db:    db,
	}
}

func (b *goleveldbBatch) CheckFlush() error {
	if len(b.Dump()) > goleveldbFlushBatch {
		if err := b.Flush(); err != nil {
			return err
		}
		b.Reset()
	}
	return nil
}

func (b *goleveldbBatch) Flush() error {
	b.db.closeMut.RLock()
	defer b.db.closeMut.RUnlock()
	if b.db.closed {
		return leveldb.ErrClosed
	}
	return b.db.convertError(b.db.Write(b.Batch, nil))
}

type snap struct {
	*leveldb.Snapshot
	db *goleveldb
}

func (s *snap) Get(key []byte) ([]byte, error) {
	val, err := s.Snapshot.Get(key, nil)
	return val, s.db.convertError(err)
}

func (s *snap) Has(key []byte) (bool, error) {
	has, err := s.Snapshot.Has(key, nil)
	return has, s.db.convertError(err)
}

func (s *snap) NewIterator(slice *util.Range) (iterator.Iterator, error) {
	return s.db.newIterator(func() iterator.Iterator {
		return s.Snapshot.NewIterator(slice, nil)
	})
}

// iter implements iterator.Iterator which allows tracking active iterators
// and aborts if the underlying database is being closed.
type iter struct {
	iterator.Iterator
	db *goleveldb
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
