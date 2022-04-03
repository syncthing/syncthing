// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// Never flush transactions smaller than this, even on Checkpoint().
	// This just needs to be just large enough to avoid flushing
	// transactions when they are super tiny, thus creating millions of tiny
	// transactions unnecessarily.
	dbFlushBatchMin = 64 << KiB
	// Once a transaction reaches this size, flush it unconditionally. This
	// should be large enough to avoid forcing a flush between Checkpoint()
	// calls in loops where we do those, so in principle just large enough
	// to hold a FileInfo plus corresponding version list and metadata
	// updates or two.
	dbFlushBatchMax = 1 << MiB
)

// leveldbBackend implements Backend on top of a leveldb
type leveldbBackend struct {
	ldb      *leveldb.DB
	closeWG  *closeWaitGroup
	location string
}

func newLeveldbBackend(ldb *leveldb.DB, location string) *leveldbBackend {
	return &leveldbBackend{
		ldb:      ldb,
		closeWG:  &closeWaitGroup{},
		location: location,
	}
}

func (b *leveldbBackend) NewReadTransaction() (ReadTransaction, error) {
	return b.newSnapshot()
}

func (b *leveldbBackend) newSnapshot() (leveldbSnapshot, error) {
	rel, err := newReleaser(b.closeWG)
	if err != nil {
		return leveldbSnapshot{}, err
	}
	snap, err := b.ldb.GetSnapshot()
	if err != nil {
		rel.Release()
		return leveldbSnapshot{}, wrapLeveldbErr(err)
	}
	return leveldbSnapshot{
		snap: snap,
		rel:  rel,
	}, nil
}

func (b *leveldbBackend) NewWriteTransaction(hooks ...CommitHook) (WriteTransaction, error) {
	rel, err := newReleaser(b.closeWG)
	if err != nil {
		return nil, err
	}
	snap, err := b.newSnapshot()
	if err != nil {
		rel.Release()
		return nil, err // already wrapped
	}

	t := &leveldbTransaction{
		leveldbSnapshot: snap,
		ldb:             b.ldb,
		batch:           new(leveldb.Batch),
		rel:             rel,
	}
	t.flusher = &batchFlusher{
		size:  func() int { return len(t.batch.Dump()) },
		empty: func() bool { return t.batch.Len() == 0 },
		writeAndReset: func() error {
			if err := t.ldb.Write(t.batch, nil); err != nil {
				return wrapLeveldbErr(err)
			}
			t.batch.Reset()
			return nil
		},
		transaction: t,
		commitHooks: hooks,
	}
	return t, nil
}

func (b *leveldbBackend) Close() error {
	b.closeWG.CloseWait()
	return wrapLeveldbErr(b.ldb.Close())
}

func (b *leveldbBackend) Get(key []byte) ([]byte, error) {
	val, err := b.ldb.Get(key, nil)
	return val, wrapLeveldbErr(err)
}

func (b *leveldbBackend) NewPrefixIterator(prefix []byte) (Iterator, error) {
	return &leveldbIterator{b.ldb.NewIterator(util.BytesPrefix(prefix), nil)}, nil
}

func (b *leveldbBackend) NewRangeIterator(first, last []byte) (Iterator, error) {
	return &leveldbIterator{b.ldb.NewIterator(&util.Range{Start: first, Limit: last}, nil)}, nil
}

func (b *leveldbBackend) Put(key, val []byte) error {
	return wrapLeveldbErr(b.ldb.Put(key, val, nil))
}

func (b *leveldbBackend) Delete(key []byte) error {
	return wrapLeveldbErr(b.ldb.Delete(key, nil))
}

func (b *leveldbBackend) Compact() error {
	// Race is detected during testing when db is closed while compaction
	// is ongoing.
	err := b.closeWG.Add(1)
	if err != nil {
		return err
	}
	defer b.closeWG.Done()
	return wrapLeveldbErr(b.ldb.CompactRange(util.Range{}))
}

func (b *leveldbBackend) Location() string {
	return b.location
}

// leveldbSnapshot implements backend.ReadTransaction
type leveldbSnapshot struct {
	snap *leveldb.Snapshot
	rel  *releaser
}

func (l leveldbSnapshot) Get(key []byte) ([]byte, error) {
	val, err := l.snap.Get(key, nil)
	return val, wrapLeveldbErr(err)
}

func (l leveldbSnapshot) NewPrefixIterator(prefix []byte) (Iterator, error) {
	return l.snap.NewIterator(util.BytesPrefix(prefix), nil), nil
}

func (l leveldbSnapshot) NewRangeIterator(first, last []byte) (Iterator, error) {
	return l.snap.NewIterator(&util.Range{Start: first, Limit: last}, nil), nil
}

func (l leveldbSnapshot) Release() {
	l.snap.Release()
	l.rel.Release()
}

// leveldbTransaction implements backend.WriteTransaction using a batch (not
// an actual leveldb transaction)
type leveldbTransaction struct {
	leveldbSnapshot
	ldb     *leveldb.DB
	batch   *leveldb.Batch
	rel     *releaser
	flusher *batchFlusher
}

func (t *leveldbTransaction) Delete(key []byte) error {
	t.batch.Delete(key)
	return t.flusher.check(dbFlushBatchMax)
}

func (t *leveldbTransaction) Put(key, val []byte) error {
	t.batch.Put(key, val)
	return t.flusher.check(dbFlushBatchMax)
}

func (t *leveldbTransaction) Checkpoint() error {
	return t.flusher.check(dbFlushBatchMin)
}

func (t *leveldbTransaction) Commit() error {
	err := wrapLeveldbErr(t.flusher.flush())
	t.leveldbSnapshot.Release()
	t.rel.Release()
	return err
}

func (t *leveldbTransaction) Release() {
	t.leveldbSnapshot.Release()
	t.rel.Release()
}

type leveldbIterator struct {
	iterator.Iterator
}

func (it *leveldbIterator) Error() error {
	return wrapLeveldbErr(it.Iterator.Error())
}

// wrapLeveldbErr wraps errors so that the backend package can recognize them
func wrapLeveldbErr(err error) error {
	switch err {
	case leveldb.ErrClosed:
		return errClosed
	case leveldb.ErrNotFound:
		return errNotFound
	}
	return err
}
