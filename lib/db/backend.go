// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	ErrNotFound = errors.New("given key not found in db")
	ErrClosed   = errors.New("db backend was closed")
)

// Backend is the interface that a database needs to implement to be used with
// Syncthing.
// It is not guaranteed that after calling Close no more calls are made to other
// methods. All returned errors due to methods being called after Close must be
// ErrClosed.
// Errors due to a given key not being found in the database, must be ErrNotFound.
type Backend interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	Put(key, val []byte) error
	Delete(key []byte) error
	NewBatch() Batch
	NewIterator(slice *util.Range) (iterator.Iterator, error)
	GetSnapshot() (Snapshot, error)
	Close()
}

// Batch is a container to check in several changes at once to the database to
// ensure a semantically consistent state.
//
// CheckFlush should check the size of the Batch write and call Flush in
// appropriate intervals (e.g. when the Batch exceeds a certain size).
//
// Flush commits the contents of the Batch to db and calls Reset on it on success.
type Batch interface {
	CheckFlush() error
	Delete(key []byte)
	Flush() error
	Len() int
	Put(key, value []byte)
	Reset()
}

// Snapshot provides a consistent view of the database at the time of creation.
// It must be released after use by calling Release.
type Snapshot interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	NewIterator(*util.Range) (iterator.Iterator, error)
	Release()
}
