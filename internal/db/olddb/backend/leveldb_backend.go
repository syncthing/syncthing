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
