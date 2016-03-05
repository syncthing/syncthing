// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/testutil"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type testingDB struct {
	*DB
	ro   *opt.ReadOptions
	wo   *opt.WriteOptions
	stor *testutil.Storage
}

func (t *testingDB) TestPut(key []byte, value []byte) error {
	return t.Put(key, value, t.wo)
}

func (t *testingDB) TestDelete(key []byte) error {
	return t.Delete(key, t.wo)
}

func (t *testingDB) TestGet(key []byte) (value []byte, err error) {
	return t.Get(key, t.ro)
}

func (t *testingDB) TestHas(key []byte) (ret bool, err error) {
	return t.Has(key, t.ro)
}

func (t *testingDB) TestNewIterator(slice *util.Range) iterator.Iterator {
	return t.NewIterator(slice, t.ro)
}

func (t *testingDB) TestClose() {
	err := t.Close()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	err = t.stor.Close()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func newTestingDB(o *opt.Options, ro *opt.ReadOptions, wo *opt.WriteOptions) *testingDB {
	stor := testutil.NewStorage()
	db, err := Open(stor, o)
	// FIXME: This may be called from outside It, which may cause panic.
	Expect(err).NotTo(HaveOccurred())
	return &testingDB{
		DB:   db,
		ro:   ro,
		wo:   wo,
		stor: stor,
	}
}

type testingTransaction struct {
	*Transaction
	ro *opt.ReadOptions
	wo *opt.WriteOptions
}

func (t *testingTransaction) TestPut(key []byte, value []byte) error {
	return t.Put(key, value, t.wo)
}

func (t *testingTransaction) TestDelete(key []byte) error {
	return t.Delete(key, t.wo)
}

func (t *testingTransaction) TestGet(key []byte) (value []byte, err error) {
	return t.Get(key, t.ro)
}

func (t *testingTransaction) TestHas(key []byte) (ret bool, err error) {
	return t.Has(key, t.ro)
}

func (t *testingTransaction) TestNewIterator(slice *util.Range) iterator.Iterator {
	return t.NewIterator(slice, t.ro)
}

func (t *testingTransaction) TestClose() {}
