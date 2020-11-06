// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/locations"
)

// CommitHook is a function that is executed before a WriteTransaction is
// committed or before it is flushed to disk, e.g. on calling CheckPoint. The
// transaction can be accessed via a closure.
type CommitHook func(WriteTransaction) error

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
// resource constraints, but this gives you a chance to decide when. If, and
// only if, calling Checkpoint will result in a partial commit/flush, the
// CommitHooks passed to Backend.NewWriteTransaction are called before
// committing. If any of those returns an error, committing is aborted and the
// error bubbled.
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
// Location returns the path to the database, as given to Open. The returned string
// is empty for a db in memory.
type Backend interface {
	Reader
	Writer
	NewReadTransaction() (ReadTransaction, error)
	NewWriteTransaction(hooks ...CommitHook) (WriteTransaction, error)
	Close() error
	Compact() error
	Location() string
}

type Tuning int

const (
	// N.b. these constants must match those in lib/config.Tuning!
	TuningAuto Tuning = iota
	TuningSmall
	TuningLarge
)

func Open(path string, tuning Tuning) (Backend, error) {
	if err := maybeCopyDatabase(path, strings.Replace(path, locations.LevelDBDir, locations.BadgerDir, 1), OpenLevelDBAuto, OpenBadger); err != nil {
		return nil, err
	}

	return OpenLevelDB(path, tuning)
}

func OpenMemory() Backend {
	return OpenLevelDBMemory()
}

type errClosed struct{}

func (*errClosed) Error() string { return "database is closed" }

type errNotFound struct{}

func (*errNotFound) Error() string { return "key not found" }

func IsClosed(err error) bool {
	var e *errClosed
	return errors.As(err, &e)
}

func IsNotFound(err error) bool {
	var e *errNotFound
	return errors.As(err, &e)
}

// releaser manages counting on top of a waitgroup
type releaser struct {
	wg   *closeWaitGroup
	once *sync.Once
}

func newReleaser(wg *closeWaitGroup) (*releaser, error) {
	if err := wg.Add(1); err != nil {
		return nil, err
	}
	return &releaser{
		wg:   wg,
		once: new(sync.Once),
	}, nil
}

func (r releaser) Release() {
	// We use the Once because we may get called multiple times from
	// Commit() and deferred Release().
	r.once.Do(func() {
		r.wg.Done()
	})
}

// closeWaitGroup behaves just like a sync.WaitGroup, but does not require
// a single routine to do the Add and Wait calls. If Add is called after
// CloseWait, it will return an error, and both are safe to be used concurrently.
type closeWaitGroup struct {
	sync.WaitGroup
	closed   bool
	closeMut sync.RWMutex
}

func (cg *closeWaitGroup) Add(i int) error {
	cg.closeMut.RLock()
	defer cg.closeMut.RUnlock()
	if cg.closed {
		return &errClosed{}
	}
	cg.WaitGroup.Add(i)
	return nil
}

func (cg *closeWaitGroup) CloseWait() {
	cg.closeMut.Lock()
	cg.closed = true
	cg.closeMut.Unlock()
	cg.WaitGroup.Wait()
}

type opener func(path string) (Backend, error)

// maybeCopyDatabase copies the database if the destination doesn't exist
// but the source does.
func maybeCopyDatabase(toPath, fromPath string, toOpen, fromOpen opener) error {
	if _, err := os.Lstat(toPath); !os.IsNotExist(err) {
		// Destination database exists (or is otherwise unavailable), do not
		// attempt to overwrite it.
		return nil
	}

	if _, err := os.Lstat(fromPath); err != nil {
		// Source database is not available, so nothing to copy
		return nil
	}

	fromDB, err := fromOpen(fromPath)
	if err != nil {
		return err
	}
	defer fromDB.Close()

	toDB, err := toOpen(toPath)
	if err != nil {
		// That's odd, but it will be handled & reported in the usual path
		// so we can ignore it here.
		return err
	}
	defer toDB.Close()

	l.Infoln("Copying database for format conversion...")
	if err := copyBackend(toDB, fromDB); err != nil {
		return err
	}

	// Move the old database out of the way to mark it as migrated.
	fromDB.Close()
	_ = os.Rename(fromPath, fromPath+".migrated."+time.Now().Format("20060102150405"))
	return nil
}

func copyBackend(to, from Backend) error {
	srcIt, err := from.NewPrefixIterator(nil)
	if err != nil {
		return err
	}
	defer srcIt.Release()

	dstTx, err := to.NewWriteTransaction()
	if err != nil {
		return err
	}
	defer dstTx.Release()

	for srcIt.Next() {
		if err := dstTx.Put(srcIt.Key(), srcIt.Value()); err != nil {
			return err
		}
	}
	if srcIt.Error() != nil {
		return err
	}
	srcIt.Release()

	return dstTx.Commit()
}
