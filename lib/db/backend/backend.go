// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"sync"
)

// The Reader interface specifies the read-only operations available on the
// main database and on read-only transactions (snapshots). Note that when
// called directly on the database handle these operations may take implicit
// transactions and performance may suffer.
type Reader interface {
	Get(key []byte) ([]byte, error)
	NewPrefixIterator(prefix []byte) (Iterator, error)
	NewRangeIterator(first, last []byte) (Iterator, error)
}

// The Writer interface specifies the mutating operations available on the
// main database and on writable transactions. Note that when called
// directly on the database handle these operations may take implicit
// transactions and performance may suffer.
type Writer interface {
	Put(key, val []byte) error
	Delete(key []byte) error
}

// The ReadTransaction interface specifies the operations on read-only
// transactions. Every ReadTransaction must be Released when no longer
// required.
type ReadTransaction interface {
	Reader
	Release()
}

// The WriteTransaction interface specifies the operations on writable
// transactions. Every WriteTransaction must be either Commited or Released
// (i.e., discarded) when no longer required. It is fine to Release an
// already Commited transaction.
type WriteTransaction interface {
	ReadTransaction
	Writer
	Commit() error
}

// The Iterator interface specifies the operations available on iterators returned by NewPrefixIterator and NewRangeIterator. Note that there are multiple ways an error may be reported: in the Value()
type Iterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Error() error
	Release()
}

type Backend interface {
	Reader
	Writer
	NewReadTransaction() (ReadTransaction, error)
	NewWriteTransaction() (WriteTransaction, error)
	Close() error
}

type Tuning int

const (
	// N.b. these constants must match those in lib/config.Tuning!
	TuningAuto Tuning = iota
	TuningSmall
	TuningLarge
)

func Open(path string, tuning Tuning) (Backend, error) {
	return OpenLevelDB(path, tuning)
}

func OpenMemory() Backend {
	return OpenLevelDBMemory()
}

type errClosed struct{}

func (errClosed) Error() string { return "database is closed" }

type errNotFound struct{}

func (errNotFound) Error() string { return "key not found" }

func IsClosed(err error) bool {
	if _, ok := err.(errClosed); ok {
		return true
	}
	if _, ok := err.(*errClosed); ok {
		return true
	}
	return false
}

func IsNotFound(err error) bool {
	if _, ok := err.(errNotFound); ok {
		return true
	}
	if _, ok := err.(*errNotFound); ok {
		return true
	}
	return false
}

// releaser manages counting on top of a waitgroup
type releaser struct {
	wg   *sync.WaitGroup
	once *sync.Once
}

func newReleaser(wg *sync.WaitGroup) *releaser {
	wg.Add(1)
	return &releaser{
		wg:   wg,
		once: new(sync.Once),
	}
}

func (r releaser) Release() {
	// We use the Once because we may get called multiple times from
	// Commit() and deferred Release().
	r.once.Do(func() {
		r.wg.Done()
	})
}
