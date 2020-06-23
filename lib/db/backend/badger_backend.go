// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"bytes"
	"errors"
	"time"

	badger "github.com/dgraph-io/badger/v2"
)

const (
	checkpointFlushMinSize = 128 << KiB
	maxCacheSize           = 64 << MiB
)

func OpenBadger(path string) (Backend, error) {
	opts := badger.DefaultOptions(path)
	opts = opts.WithMaxCacheSize(maxCacheSize).WithCompactL0OnClose(false)
	opts.Logger = nil
	return openBadger(opts)
}

func OpenBadgerMemory() Backend {
	opts := badger.DefaultOptions("").WithInMemory(true)
	opts.Logger = nil
	backend, err := openBadger(opts)
	if err != nil {
		// Opening in-memory should never be able to fail, and is anyway
		// used just by tests.
		panic(err)
	}
	return backend
}

func openBadger(opts badger.Options) (Backend, error) {
	// XXX: We should find good values for memory utilization in the "small"
	// and "large" cases we support for LevelDB. Some notes here:
	// https://github.com/dgraph-io/badger/tree/v2.0.3#memory-usage
	bdb, err := badger.Open(opts)
	if err != nil {
		return nil, wrapBadgerErr(err)
	}
	return &badgerBackend{
		bdb:     bdb,
		closeWG: &closeWaitGroup{},
	}, nil
}

// badgerBackend implements Backend on top of a badger
type badgerBackend struct {
	bdb     *badger.DB
	closeWG *closeWaitGroup
}

func (b *badgerBackend) NewReadTransaction() (ReadTransaction, error) {
	rel, err := newReleaser(b.closeWG)
	if err != nil {
		return nil, err
	}
	return badgerSnapshot{
		txn: b.bdb.NewTransaction(false),
		rel: rel,
	}, nil
}

func (b *badgerBackend) NewWriteTransaction() (WriteTransaction, error) {
	rel1, err := newReleaser(b.closeWG)
	if err != nil {
		return nil, err
	}
	rel2, err := newReleaser(b.closeWG)
	if err != nil {
		rel1.Release()
		return nil, err
	}

	// We use two transactions here to preserve the property that our
	// leveldb wrapper has, that writes in a transaction are completely
	// invisible until it's committed, even inside that same transaction.
	rtxn := b.bdb.NewTransaction(false)
	wtxn := b.bdb.NewTransaction(true)
	return &badgerTransaction{
		badgerSnapshot: badgerSnapshot{
			txn: rtxn,
			rel: rel1,
		},
		txn: wtxn,
		bdb: b.bdb,
		rel: rel2,
	}, nil
}

func (b *badgerBackend) Close() error {
	b.closeWG.CloseWait()
	return wrapBadgerErr(b.bdb.Close())
}

func (b *badgerBackend) Get(key []byte) ([]byte, error) {
	if err := b.closeWG.Add(1); err != nil {
		return nil, err
	}
	defer b.closeWG.Done()

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
	if err := b.closeWG.Add(1); err != nil {
		return nil, err
	}

	txn := b.bdb.NewTransaction(false)
	it := badgerPrefixIterator(txn, prefix)
	it.releaseFn = func() {
		defer b.closeWG.Done()
		txn.Discard()
	}
	return it, nil
}

func (b *badgerBackend) NewRangeIterator(first, last []byte) (Iterator, error) {
	if err := b.closeWG.Add(1); err != nil {
		return nil, err
	}

	txn := b.bdb.NewTransaction(false)
	it := badgerRangeIterator(txn, first, last)
	it.releaseFn = func() {
		defer b.closeWG.Done()
		txn.Discard()
	}
	return it, nil
}

func (b *badgerBackend) Put(key, val []byte) error {
	if err := b.closeWG.Add(1); err != nil {
		return err
	}
	defer b.closeWG.Done()

	txn := b.bdb.NewTransaction(true)
	if err := txn.Set(key, val); err != nil {
		txn.Discard()
		return wrapBadgerErr(err)
	}
	return wrapBadgerErr(txn.Commit())
}

func (b *badgerBackend) Delete(key []byte) error {
	if err := b.closeWG.Add(1); err != nil {
		return err
	}
	defer b.closeWG.Done()

	txn := b.bdb.NewTransaction(true)
	if err := txn.Delete(key); err != nil {
		txn.Discard()
		return wrapBadgerErr(err)
	}
	return wrapBadgerErr(txn.Commit())
}

func (b *badgerBackend) Compact() error {
	if err := b.closeWG.Add(1); err != nil {
		return err
	}
	defer b.closeWG.Done()

	// This weird looking loop is as recommended in the README
	// (https://github.com/dgraph-io/badger/tree/v2.0.3#garbage-collection).
	// Basically, the RunValueLogGC will pick some promising thing to
	// garbage collect at random and return nil if it improved the
	// situation, then return ErrNoRewrite when there is nothing more to GC.
	// The 0.5 is the discard ratio, for which the method docs say they
	// "recommend setting discardRatio to 0.5, thus indicating that a file
	// be rewritten if half the space can be discarded".
	var err error
	t0 := time.Now()
	for err == nil {
		if time.Since(t0) > time.Hour {
			l.Warnln("Database compaction is taking a long time, performance may be impacted. Consider investigating and/or opening an issue if this warning repeats.")
			t0 = time.Now()
		}
		err = b.bdb.RunValueLogGC(0.5)
	}

	if errors.Is(err, badger.ErrNoRewrite) {
		// GC did nothing, because nothing needed to be done
		return nil
	}
	if errors.Is(err, badger.ErrRejected) {
		// GC was already running (could possibly happen), or the database
		// is closed (can't happen).
		return nil
	}
	if errors.Is(err, badger.ErrGCInMemoryMode) {
		// GC in in-memory mode, which is fine.
		return nil
	}
	return err
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

func (t *badgerTransaction) Checkpoint(preFlush ...func() error) error {
	if t.size < checkpointFlushMinSize {
		return nil
	}
	for _, hook := range preFlush {
		if err := hook(); err != nil {
			return err
		}
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
	err       error
}

func (i *badgerIterator) Next() bool {
	if i.err != nil {
		return false
	}
	for {
		if !i.didSeek {
			if i.first != nil {
				// Range iterator
				i.it.Seek(i.first)
			} else {
				// Prefix iterator
				i.it.Seek(i.prefix)
			}
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
		if bytes.Compare(key, i.last) > 0 {
			// Key is after range last
			return false
		}
		return true
	}
}

func (i *badgerIterator) Key() []byte {
	if i.err != nil {
		return nil
	}
	return i.it.Item().Key()
}

func (i *badgerIterator) Value() []byte {
	if i.err != nil {
		return nil
	}
	val, err := i.it.Item().ValueCopy(nil)
	if err != nil {
		i.err = err
	}
	return val
}

func (i *badgerIterator) Error() error {
	return wrapBadgerErr(i.err)
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
		return &errClosed{}
	}
	if err == badger.ErrKeyNotFound {
		return &errNotFound{}
	}
	return err
}

func badgerPrefixIterator(txn *badger.Txn, prefix []byte) *badgerIterator {
	it := iteratorForPrefix(txn, prefix)
	return &badgerIterator{it: it, prefix: prefix}
}

func badgerRangeIterator(txn *badger.Txn, first, last []byte) *badgerIterator {
	prefix := commonPrefix(first, last)
	it := iteratorForPrefix(txn, prefix)
	return &badgerIterator{it: it, prefix: prefix, first: first, last: last}
}

func iteratorForPrefix(txn *badger.Txn, prefix []byte) *badger.Iterator {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	return txn.NewIterator(opts)
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
