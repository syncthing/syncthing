// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"io"
	"os"
	"sync"

	"github.com/cockroachdb/pebble"
)

// pebbleBackend implements Backend on top of a pebble
type pebbleBackend struct {
	db        *pebble.DB
	closeWG   *closeWaitGroup
	location  string
	temporary bool
}

func newPebbleBackend(db *pebble.DB, location string, temporary bool) *pebbleBackend {
	return &pebbleBackend{
		db:        db,
		closeWG:   &closeWaitGroup{},
		location:  location,
		temporary: temporary,
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
	t := &pebbleTransaction{
		pebbleSnapshot: snap,
		batch:          b.db.NewBatch(),
		rel:            rel,
	}
	t.flusher = &batchFlusher{
		size:  func() int { return len(t.batch.Repr()) },
		empty: func() bool { return t.batch.Empty() },
		writeAndReset: func() error {
			if err := t.batch.Commit(nil); err != nil {
				return wrapPebbleErr(err)
			}
			t.batch.Reset()
			return nil
		},
		transaction: t,
		commitHooks: hooks,
	}
	return t, nil
}

func (b *pebbleBackend) Close() error {
	if b.closeWG.Closed() {
		return errClosed
	}
	b.closeWG.CloseWait()
	err := b.db.Close()
	if b.temporary {
		delErr := os.RemoveAll(b.location)
		if err == nil && delErr != nil {
			return delErr
		}
	}
	return wrapPebbleErr(err)
}

func (b *pebbleBackend) Get(key []byte) ([]byte, error) {
	if b.closeWG.Closed() {
		return nil, errClosed
	}
	return pebbleHandleGet(b.db.Get(key))
}

func (b *pebbleBackend) NewPrefixIterator(prefix []byte) (Iterator, error) {
	if b.closeWG.Closed() {
		return nil, errClosed
	}
	if len(prefix) == 0 {
		return newPebbleIterator(b.db.NewIter(nil)), nil
	}
	after := make([]byte, len(prefix))
	copy(after, prefix)
	for i := len(prefix) - 1; i >= 0; i-- {
		after[i] += 1
		if after[i] > 0 {
			after = after[:i+1]
			break
		}
	}
	return b.NewRangeIterator(prefix, after)
}

func (b *pebbleBackend) NewRangeIterator(first, last []byte) (Iterator, error) {
	if b.closeWG.Closed() {
		return nil, errClosed
	}
	return newPebbleIterator(b.db.NewIter(&pebble.IterOptions{LowerBound: first, UpperBound: last})), nil
}

func (b *pebbleBackend) Put(key, val []byte) error {
	if b.closeWG.Closed() {
		return errClosed
	}
	return wrapPebbleErr(b.db.Set(key, val, nil))
}

func (b *pebbleBackend) Delete(key []byte) error {
	if b.closeWG.Closed() {
		return errClosed
	}
	return wrapPebbleErr(b.db.Delete(key, nil))
}

func (b *pebbleBackend) Compact() error {
	// Race is detected during testing when db is closed while compaction
	// is ongoing.
	err := b.closeWG.Add(1)
	if err != nil {
		return err
	}
	defer b.closeWG.Done()
	it := b.db.NewIter(nil)
	it.Last()
	end := it.Key()
	if err := it.Close(); err != nil {
		return wrapPebbleErr(err)
	}
	return wrapPebbleErr(b.db.Compact(nil, end, true))
}

func (b *pebbleBackend) Location() string {
	return b.location
}

// pebbleSnapshot implements backend.ReadTransaction
type pebbleSnapshot struct {
	snap *pebble.Snapshot
	rel  *releaser
}

func (s *pebbleSnapshot) Get(key []byte) ([]byte, error) {
	return pebbleHandleGet(s.snap.Get(key))
}

func (s *pebbleSnapshot) NewPrefixIterator(prefix []byte) (Iterator, error) {
	if len(prefix) == 0 {
		return newPebbleIterator(s.snap.NewIter(nil)), nil
	}
	after := make([]byte, len(prefix))
	copy(after, prefix)
	for i := len(prefix) - 1; i >= 0; i-- {
		after[i] += 1
		if after[i] > 0 {
			after = after[:i+1]
			break
		}
	}
	return s.NewRangeIterator(prefix, after)
}

func (s *pebbleSnapshot) NewRangeIterator(first, last []byte) (Iterator, error) {
	return newPebbleIterator(s.snap.NewIter(&pebble.IterOptions{LowerBound: first, UpperBound: last})), nil
}

func (s *pebbleSnapshot) Release() {
	// api says "no error here", so lets ignore it. It likely surfaced
	// anyway on the iterator or will surface whenever the api is called next.
	if !s.rel.Released() {
		_ = s.snap.Close()
		s.rel.Release()
	}
}

// pebbleTransaction implements backend.WriteTransaction using a batch
type pebbleTransaction struct {
	*pebbleSnapshot
	batch   *pebble.Batch
	rel     *releaser
	flusher *batchFlusher
}

func (t *pebbleTransaction) Delete(key []byte) error {
	if err := t.batch.Delete(key, nil); err != nil {
		return wrapPebbleErr(err)
	}
	return t.flusher.check(dbFlushBatchMax)
}

func (t *pebbleTransaction) Put(key, val []byte) error {
	if err := t.batch.Set(key, val, nil); err != nil {
		return wrapPebbleErr(err)
	}
	return t.flusher.check(dbFlushBatchMax)
}

func (t *pebbleTransaction) Checkpoint() error {
	return t.flusher.check(dbFlushBatchMin)
}

func (t *pebbleTransaction) Commit() error {
	err := wrapPebbleErr(t.flusher.flush())
	t.pebbleSnapshot.Release()
	t.rel.Release()
	return err
}

func (t *pebbleTransaction) Release() {
	t.pebbleSnapshot.Release()
	t.rel.Release()
}

func pebbleHandleGet(val []byte, closer io.Closer, err error) ([]byte, error) {
	if err != nil {
		return nil, wrapPebbleErr(err)
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out, wrapPebbleErr(closer.Close())
}

type pebbleIterator struct {
	it       *pebble.Iterator
	mut      sync.Mutex
	released bool
	err      error
}

func newPebbleIterator(it *pebble.Iterator) *pebbleIterator {
	// Iterator needs to be positioned before it can be used (i.e. Next called)
	it.First()
	return &pebbleIterator{
		it: it,
	}
}

func (it *pebbleIterator) Next() bool {
	return it.it.Next()
}

func (it *pebbleIterator) Key() []byte {
	return it.it.Key()
}

func (it *pebbleIterator) Value() []byte {
	return it.it.Value()
}

func (it *pebbleIterator) Error() error {
	it.mut.Lock()
	defer it.mut.Unlock()
	if it.released {
		return it.err
	}
	return wrapPebbleErr(it.it.Error())
}

func (it *pebbleIterator) Release() {
	it.mut.Lock()
	if !it.released {
		it.err = wrapPebbleErr(it.it.Close())
		it.released = true
	}
	it.mut.Unlock()
}

// wrapPebbleErr wraps errors so that the backend package can recognize them
func wrapPebbleErr(err error) error {
	switch err {
	case pebble.ErrClosed:
		return errClosed
	case pebble.ErrNotFound:
		return errNotFound
	}
	return err
}
