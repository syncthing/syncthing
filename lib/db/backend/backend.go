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
// transactions. Every ReadTransaction must be released when no longer
// required.
type ReadTransaction interface {
	Reader
	Release()
}

// The WriteTransaction interface specifies the operations on writable
// transactions. Every WriteTransaction must be either committed or released
// (i.e., discarded) when no longer required. No further operations must be
// performed after release or commit (regardless of whether commit succeeded),
// with one exception -- it's fine to release an already committed or released
// transaction.
//
// A Checkpoint is a potential partial commit of the transaction so far, for
// purposes of saving memory when transactions are in-RAM. Note that
// transactions may be checkpointed *anyway* even if this is not called, due to
// resource constraints, but this gives you a chance to decide when.
type WriteTransaction interface {
	ReadTransaction
	Writer
	Checkpoint() error
	Commit() error
}

// The Iterator interface specifies the operations available on iterators
// returned by NewPrefixIterator and NewRangeIterator. The iterator pattern
// is to loop while Next returns true, then check Error after the loop. Next
// will return false when iteration is complete (Error() == nil) or when
// there is an error preventing iteration, which is then returned by
// Error(). For example:
//
//     it, err := db.NewPrefixIterator(nil)
//     if err != nil {
//         // problem preventing iteration
//     }
//     defer it.Release()
//     for it.Next() {
//         // ...
//     }
//     if err := it.Error(); err != nil {
//         // there was a database problem while iterating
//     }
//
// An iterator must be Released when no longer required. The Error method
// can be called either before or after Release with the same results. If an
// iterator was created in a transaction (whether read-only or write) it
// must be released before the transaction is released (or committed).
type Iterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Error() error
	Release()
}

// The Backend interface represents the main database handle. It supports
// both read/write operations and opening read-only or writable
// transactions. Depending on the actual implementation, individual
// read/write operations may be implicitly wrapped in transactions, making
// them perform quite badly when used repeatedly. For bulk operations,
// consider always using a transaction of the appropriate type. The
// transaction isolation level is "read committed" - there are no dirty
// reads.
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
