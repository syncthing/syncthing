// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"bytes"
	"sync"

	badger "github.com/dgraph-io/badger/v2"
)

const (
	checkpointFlushMinSize = 1 << 20
)

func OpenBadger(path string) (Backend, error) {
	opts := badger.DefaultOptions(path + ".badger") // temporary path safety
	opts = opts.WithMaxCacheSize(64 << MiB).WithCompactL0OnClose(false)
	opts.Logger = nil
	bdb, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &badgerBackend{
		bdb:    bdb,
		closed: make(chan struct{}),
	}, nil
}

func OpenBadgerMemory() Backend {
	opts := badger.DefaultOptions("").WithInMemory(true)
	opts.Logger = nil
	bdb, err := badger.Open(opts)
	if err != nil {
		panic(err)
	}
	return &badgerBackend{
		bdb:    bdb,
		closed: make(chan struct{}),
	}
}

// badgerBackend implements Backend on top of a badger
type badgerBackend struct {
	bdb     *badger.DB
	closeWG sync.WaitGroup
	closed  chan struct{}
}

func (b *badgerBackend) NewReadTransaction() (ReadTransaction, error) {
	return badgerSnapshot{
		txn: b.bdb.NewTransaction(false),
		rel: newReleaser(&b.closeWG),
	}, nil
}

func (b *badgerBackend) NewWriteTransaction() (WriteTransaction, error) {
	// We use two transactions here to preserve the property that our
	// leveldb wrapper has, that writes in a transaction are completely
	// invisible until it's committed, even inside that same transaction.
	rtxn := b.bdb.NewTransaction(false)
	wtxn := b.bdb.NewTransaction(true)
	return &badgerTransaction{
		badgerSnapshot: badgerSnapshot{
			txn: rtxn,
			rel: newReleaser(&b.closeWG),
		},
		txn: wtxn,
		bdb: b.bdb,
		rel: newReleaser(&b.closeWG),
	}, nil
}

func (b *badgerBackend) Close() error {
	b.closeWG.Wait()
	err := b.bdb.Close()
	close(b.closed)
	return wrapBadgerErr(err)
}

func (b *badgerBackend) Get(key []byte) ([]byte, error) {
	txn := b.bdb.NewTransaction(false)
	defer txn.Discard()
	item, err := txn.Get(key)
	if err != nil {
		return nil, wrapBadgerErr(err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		return nil, wrapBadgerErr(err)
	}
	return val, nil
}

func (b *badgerBackend) NewPrefixIterator(prefix []byte) (Iterator, error) {
	select {
	case <-b.closed:
		return nil, errClosed{}
	default:
	}

	txn := b.bdb.NewTransaction(false)
	it := badgerPrefixIterator(txn, prefix)
	it.releaseFn = func() {
		txn.Discard()
	}
	return it, nil
}

func (b *badgerBackend) NewRangeIterator(first, last []byte) (Iterator, error) {
	select {
	case <-b.closed:
		return nil, errClosed{}
	default:
	}

	txn := b.bdb.NewTransaction(false)
	it := badgerRangeIterator(txn, first, last)
	it.releaseFn = func() {
		txn.Discard()
	}
	return it, nil
}

func (b *badgerBackend) Put(key, val []byte) error {
	select {
	case <-b.closed:
		return errClosed{}
	default:
	}

	txn := b.bdb.NewTransaction(true)
	if err := txn.Set(key, val); err != nil {
		txn.Discard()
		return wrapBadgerErr(err)
	}
	return wrapBadgerErr(txn.Commit())
}

func (b *badgerBackend) Delete(key []byte) error {
	select {
	case <-b.closed:
		return errClosed{}
	default:
	}

	txn := b.bdb.NewTransaction(true)
	if err := txn.Delete(key); err != nil {
		txn.Discard()
		return wrapBadgerErr(err)
	}
	return wrapBadgerErr(txn.Commit())
}

func (b *badgerBackend) Compact() error {
	select {
	case <-b.closed:
		return errClosed{}
	default:
	}

	// XXX: check if this is appropriate or if we also need Flatten and
	// whatnot. Also it apparently returns an error if the GC didn't result
	// in anything being freed...
	_ = b.bdb.RunValueLogGC(0.5)
	return nil
}

// badgerSnapshot implements backend.ReadTransaction
type badgerSnapshot struct {
	txn *badger.Txn
	rel *releaser
}

func (l badgerSnapshot) Get(key []byte) ([]byte, error) {
	item, err := l.txn.Get(key)
	if err != nil {
		return nil, wrapBadgerErr(err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		return nil, wrapBadgerErr(err)
	}
	return val, nil
}

func (l badgerSnapshot) NewPrefixIterator(prefix []byte) (Iterator, error) {
	return badgerPrefixIterator(l.txn, prefix), nil
}

func (l badgerSnapshot) NewRangeIterator(first, last []byte) (Iterator, error) {
	return badgerRangeIterator(l.txn, first, last), nil
}

func (l badgerSnapshot) Release() {
	defer l.rel.Release()
	l.txn.Discard()
}

// badgerTransaction implements backend.WriteTransaction using a batch (not
// an actual badger transaction)
type badgerTransaction struct {
	badgerSnapshot
	txn  *badger.Txn
	bdb  *badger.DB
	rel  *releaser
	size int
}

func (t *badgerTransaction) Delete(key []byte) error {
	t.size += len(key)
	kc := make([]byte, len(key))
	copy(kc, key)
	return t.transactionRetried(func(txn *badger.Txn) error {
		return txn.Delete(kc)
	})
}

func (t *badgerTransaction) Put(key, val []byte) error {
	t.size += len(key) + len(val)
	kc := make([]byte, len(key))
	copy(kc, key)
	vc := make([]byte, len(val))
	copy(vc, val)
	return t.transactionRetried(func(txn *badger.Txn) error {
		return txn.Set(kc, vc)
	})
}

// transactionRetried performs the given operation in the current
// transaction, with commit and retry if Badger says the transaction has
// grown too large.
func (t *badgerTransaction) transactionRetried(fn func(*badger.Txn) error) error {
	if err := fn(t.txn); err == badger.ErrTxnTooBig {
		if err := t.txn.Commit(); err != nil {
			return wrapBadgerErr(err)
		}
		t.size = 0
		t.txn = t.bdb.NewTransaction(true)
		return wrapBadgerErr(fn(t.txn))
	} else if err != nil {
		return wrapBadgerErr(err)
	}
	return nil
}

func (t *badgerTransaction) Commit() error {
	defer t.rel.Release()
	defer t.badgerSnapshot.Release()
	return wrapBadgerErr(t.txn.Commit())
}

func (t *badgerTransaction) Checkpoint() error {
	if t.size < checkpointFlushMinSize {
		return nil
	}
	err := t.txn.Commit()
	if err == nil {
		t.size = 0
		t.txn = t.bdb.NewTransaction(true)
	}
	return wrapBadgerErr(err)
}

func (t *badgerTransaction) Release() {
	defer t.rel.Release()
	defer t.badgerSnapshot.Release()
	t.txn.Discard()
}

type badgerIterator struct {
	it        *badger.Iterator
	prefix    []byte
	first     []byte
	last      []byte
	releaseFn func()
	didSeek   bool
}

func (i *badgerIterator) Next() bool {
	for {
		if !i.didSeek {
			i.it.Seek(i.prefix)
			i.didSeek = true
		} else {
			i.it.Next()
		}

		if !i.it.ValidForPrefix(i.prefix) {
			// Done
			return false
		}
		if i.first == nil && i.last == nil {
			// No range checks required
			return true
		}

		key := i.it.Item().Key()
		if bytes.Compare(key, i.first) < 0 {
			// Key is before range first
			continue
		}
		if bytes.Compare(key, i.last) > 0 {
			// Key is after range last
			return false
		}
		return true
	}
}

func (i *badgerIterator) Key() []byte {
	return i.it.Item().Key()
}

func (i *badgerIterator) Value() []byte {
	val, err := i.it.Item().ValueCopy(nil)
	if err != nil {
		panic("unexpected iteration failure: " + err.Error())
	}
	return val
}

func (i *badgerIterator) Error() error {
	return nil // ???
}

func (i *badgerIterator) Release() {
	i.it.Close()
	if i.releaseFn != nil {
		i.releaseFn()
	}
}

// wrapBadgerErr wraps errors so that the backend package can recognize them
func wrapBadgerErr(err error) error {
	if err == nil {
		return nil
	}
	if err == badger.ErrDiscardedTxn {
		return errClosed{}
	}
	if err == badger.ErrKeyNotFound {
		return errNotFound{}
	}
	return err
}

func badgerPrefixIterator(txn *badger.Txn, prefix []byte) *badgerIterator {
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	bpit := &badgerIterator{it: it, prefix: prefix}
	return bpit
}

func badgerRangeIterator(txn *badger.Txn, first, last []byte) *badgerIterator {
	prefix := commonPrefix(first, last)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	return &badgerIterator{it: it, prefix: prefix, first: first, last: last}
}

func commonPrefix(a, b []byte) []byte {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	prefix := make([]byte, 0, minLen)
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			break
		}
		prefix = append(prefix, a[i])
	}
	return prefix
}
