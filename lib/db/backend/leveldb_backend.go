// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// Never flush transactions smaller than this, even on Checkpoint()
	dbFlushBatchMin = 1 << MiB
	// Once a transaction reaches this size, flush it unconditionally.
	dbFlushBatchMax = 128 << MiB
)

// leveldbBackend implements Backend on top of a leveldb
type leveldbBackend struct {
	ldb     *leveldb.DB
	closeWG sync.WaitGroup
}

func (b *leveldbBackend) NewReadTransaction() (ReadTransaction, error) {
	return b.newSnapshot()
}

func (b *leveldbBackend) newSnapshot() (leveldbSnapshot, error) {
	snap, err := b.ldb.GetSnapshot()
	if err != nil {
		return leveldbSnapshot{}, wrapLeveldbErr(err)
	}
	return leveldbSnapshot{
		snap: snap,
		rel:  newReleaser(&b.closeWG),
	}, nil
}

func (b *leveldbBackend) NewWriteTransaction() (WriteTransaction, error) {
	snap, err := b.newSnapshot()
	if err != nil {
		return nil, err // already wrapped
	}
	return &leveldbTransaction{
		leveldbSnapshot: snap,
		ldb:             b.ldb,
		batch:           new(leveldb.Batch),
		rel:             newReleaser(&b.closeWG),
	}, nil
}

func (b *leveldbBackend) Close() error {
	b.closeWG.Wait()
	return wrapLeveldbErr(b.ldb.Close())
}

func (b *leveldbBackend) Get(key []byte) ([]byte, error) {
	val, err := b.ldb.Get(key, nil)
	return val, wrapLeveldbErr(err)
}

func (b *leveldbBackend) NewPrefixIterator(prefix []byte) (Iterator, error) {
	return b.ldb.NewIterator(util.BytesPrefix(prefix), nil), nil
}

func (b *leveldbBackend) NewRangeIterator(first, last []byte) (Iterator, error) {
	return b.ldb.NewIterator(&util.Range{Start: first, Limit: last}, nil), nil
}

func (b *leveldbBackend) Put(key, val []byte) error {
	return wrapLeveldbErr(b.ldb.Put(key, val, nil))
}

func (b *leveldbBackend) Delete(key []byte) error {
	return wrapLeveldbErr(b.ldb.Delete(key, nil))
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
	ldb   *leveldb.DB
	batch *leveldb.Batch
	rel   *releaser
}

func (t *leveldbTransaction) Delete(key []byte) error {
	t.batch.Delete(key)
	return t.checkFlush(dbFlushBatchMax)
}

func (t *leveldbTransaction) Put(key, val []byte) error {
	t.batch.Put(key, val)
	return t.checkFlush(dbFlushBatchMax)
}

func (t *leveldbTransaction) Checkpoint() error {
	return t.checkFlush(dbFlushBatchMin)
}

func (t *leveldbTransaction) Commit() error {
	err := wrapLeveldbErr(t.flush())
	t.leveldbSnapshot.Release()
	t.rel.Release()
	return err
}

func (t *leveldbTransaction) Release() {
	t.leveldbSnapshot.Release()
	t.rel.Release()
}

// checkFlush flushes and resets the batch if its size exceeds the given size.
func (t *leveldbTransaction) checkFlush(size int) error {
	if len(t.batch.Dump()) < size {
		return nil
	}
	return t.flush()
}

func (t *leveldbTransaction) flush() error {
	if t.batch.Len() == 0 {
		return nil
	}
	if err := t.ldb.Write(t.batch, nil); err != nil {
		return wrapLeveldbErr(err)
	}
	t.batch.Reset()
	return nil
}

// wrapLeveldbErr wraps errors so that the backend package can recognize them
func wrapLeveldbErr(err error) error {
	if err == nil {
		return nil
	}
	if err == leveldb.ErrClosed {
		return errClosed{}
	}
	if err == leveldb.ErrNotFound {
		return errNotFound{}
	}
	return err
}
