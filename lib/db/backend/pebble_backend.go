// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"github.com/cockroachdb/pebble"
)

func OpenPebble(location string) (Backend, error) {
	db, err := pebble.Open(location, nil)
	if err != nil {
		return nil, err
	}
	return newPebbleBackend(db, location), nil
}

// pebbleBackend implements Backend on top of a pebble
type pebbleBackend struct {
	db       *pebble.DB
	closeWG  *closeWaitGroup
	location string
	closed   chan struct{}
}

func newPebbleBackend(db *pebble.DB, location string) *pebbleBackend {
	return &pebbleBackend{
		db:       db,
		closeWG:  &closeWaitGroup{},
		location: location,
		closed:   make(chan struct{}),
	}
}

func (b *pebbleBackend) NewReadTransaction() (ReadTransaction, error) {
	return b.newSnapshot()
}

func (b *pebbleBackend) newSnapshot() (*pebbleSnapshot, error) {
	rel, err := newReleaser(b.closeWG)
	if err != nil {
		return nil, err
	}
	snap := b.db.NewSnapshot()
	return &pebbleSnapshot{
		snap: snap,
		rel:  rel,
	}, nil
}

func (b *pebbleBackend) NewWriteTransaction(hooks ...CommitHook) (WriteTransaction, error) {
	rel, err := newReleaser(b.closeWG)
	if err != nil {
		return nil, err
	}
	snap, err := b.newSnapshot()
	if err != nil {
		rel.Release()
		return nil, err // already wrapped
	}
	return &pebbleTransaction{
		pebbleSnapshot: snap,
		ldb:            b.db,
		batch:          new(pebble.Batch),
		rel:            rel,
		commitHooks:    hooks,
		inFlush:        false,
	}, nil
}

func (b *pebbleBackend) Close() error {
	select {
	case <-b.closed:
		return errClosed
	default:
		close(b.closed)
	}
	b.closeWG.CloseWait()
	return wrappebbleErr(b.db.Close())
}

func (b *pebbleBackend) Get(key []byte) ([]byte, error) {
	select {
	case <-b.closed:
		return nil, errClosed
	default:
	}

	val, clo, err := b.db.Get(key)
	if err != nil {
		return nil, wrappebbleErr(err)
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	clo.Close()
	return cp, nil
}

func (b *pebbleBackend) NewPrefixIterator(prefix []byte) (Iterator, error) {
	select {
	case <-b.closed:
		return nil, errClosed
	default:
	}

	if len(prefix) == 0 {
		it, err := b.db.NewIter(nil)
		if err != nil {
			return nil, err
		}
		return &pebbleIterator{Iterator: it}, nil
	}

	last := make([]byte, len(prefix))
	copy(last, prefix)
	for i := len(last) - 1; i >= 0; i-- {
		if last[i] < 0xff {
			last[i]++
			break
		}
	}
	return b.NewRangeIterator(prefix, last)
}

func (b *pebbleBackend) NewRangeIterator(first, last []byte) (Iterator, error) {
	select {
	case <-b.closed:
		return nil, errClosed
	default:
	}

	it, err := b.db.NewIter(&pebble.IterOptions{LowerBound: first, UpperBound: last})
	if err != nil {
		return nil, err
	}
	return &pebbleIterator{Iterator: it}, nil
}

func (b *pebbleBackend) Put(key, val []byte) error {
	select {
	case <-b.closed:
		return errClosed
	default:
	}

	return wrappebbleErr(b.db.Set(key, val, nil))
}

func (b *pebbleBackend) Delete(key []byte) error {
	select {
	case <-b.closed:
		return errClosed
	default:
	}

	return wrappebbleErr(b.db.Delete(key, nil))
}

func (b *pebbleBackend) Compact() error {
	return nil
}

func (b *pebbleBackend) Location() string {
	return b.location
}

// pebbleSnapshot implements backend.ReadTransaction
type pebbleSnapshot struct {
	snap   *pebble.Snapshot
	rel    *releaser
	closed bool
}

func (l pebbleSnapshot) Get(key []byte) ([]byte, error) {
	val, clo, err := l.snap.Get(key)
	if err != nil {
		return nil, wrappebbleErr(err)
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	clo.Close()
	return cp, nil
}

func (l pebbleSnapshot) NewPrefixIterator(prefix []byte) (Iterator, error) {
	if len(prefix) == 0 {
		it, err := l.snap.NewIter(nil)
		if err != nil {
			return nil, err
		}
		return &pebbleIterator{Iterator: it}, nil
	}

	last := make([]byte, len(prefix))
	copy(last, prefix)
	for i := len(last) - 1; i >= 0; i-- {
		if last[i] < 0xff {
			last[i]++
			break
		}
	}
	return l.NewRangeIterator(prefix, last)
}

func (l pebbleSnapshot) NewRangeIterator(first, last []byte) (Iterator, error) {
	it, err := l.snap.NewIter(&pebble.IterOptions{LowerBound: first, UpperBound: last})
	if err != nil {
		return nil, err
	}
	return &pebbleIterator{Iterator: it}, nil
}

func (l *pebbleSnapshot) Release() {
	if l.closed {
		return
	}
	l.closed = true
	l.snap.Close()
	l.rel.Release()
}

// pebbleTransaction implements backend.WriteTransaction using a batch (not
// an actual pebble transaction)
type pebbleTransaction struct {
	*pebbleSnapshot
	ldb         *pebble.DB
	batch       *pebble.Batch
	rel         *releaser
	commitHooks []CommitHook
	inFlush     bool
	closed      bool
}

func (t *pebbleTransaction) Delete(key []byte) error {
	t.batch.Delete(key, nil)
	return t.checkFlush(dbFlushBatchMax)
}

func (t *pebbleTransaction) Put(key, val []byte) error {
	t.batch.Set(key, val, nil)
	return t.checkFlush(dbFlushBatchMax)
}

func (t *pebbleTransaction) Checkpoint() error {
	return t.checkFlush(dbFlushBatchMin)
}

func (t *pebbleTransaction) Commit() error {
	err := wrappebbleErr(t.flush())
	t.pebbleSnapshot.Release()
	t.rel.Release()
	return err
}

func (t *pebbleTransaction) Release() {
	if t.closed {
		return
	}
	t.closed = true
	t.pebbleSnapshot.Release()
	t.rel.Release()
}

// checkFlush flushes and resets the batch if its size exceeds the given size.
func (t *pebbleTransaction) checkFlush(size int) error {
	// Hooks might put values in the database, which triggers a checkFlush which might trigger a flush,
	// which might trigger the hooks.
	// Don't recurse...
	if t.inFlush || len(t.batch.Repr()) < size {
		return nil
	}
	return t.flush()
}

func (t *pebbleTransaction) flush() error {
	t.inFlush = true
	defer func() { t.inFlush = false }()

	for _, hook := range t.commitHooks {
		if err := hook(t); err != nil {
			return err
		}
	}
	if t.batch.Len() == 0 {
		return nil
	}
	if err := t.ldb.Apply(t.batch, nil); err != nil {
		return wrappebbleErr(err)
	}
	t.batch.Reset()
	return nil
}

type pebbleIterator struct {
	*pebble.Iterator
	firstDone bool
	closed    bool
}

func (it *pebbleIterator) Next() bool {
	if !it.firstDone {
		it.firstDone = true
		return it.Iterator.First()
	}
	return it.Iterator.Next()
}

func (it *pebbleIterator) Error() error {
	return wrappebbleErr(it.Iterator.Error())
}

func (it *pebbleIterator) Release() {
	if it.closed {
		return
	}
	it.closed = true
	it.Iterator.Close()
}

// wrappebbleErr wraps errors so that the backend package can recognize them
func wrappebbleErr(err error) error {
	switch err {
	case pebble.ErrClosed:
		return errClosed
	case pebble.ErrNotFound:
		return errNotFound
	}
	return err
}
