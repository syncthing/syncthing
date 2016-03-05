// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testutil

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type DB interface{}

type Put interface {
	TestPut(key []byte, value []byte) error
}

type Delete interface {
	TestDelete(key []byte) error
}

type Find interface {
	TestFind(key []byte) (rkey, rvalue []byte, err error)
}

type Get interface {
	TestGet(key []byte) (value []byte, err error)
}

type Has interface {
	TestHas(key []byte) (ret bool, err error)
}

type NewIterator interface {
	TestNewIterator(slice *util.Range) iterator.Iterator
}

type DBAct int

func (a DBAct) String() string {
	switch a {
	case DBNone:
		return "none"
	case DBPut:
		return "put"
	case DBOverwrite:
		return "overwrite"
	case DBDelete:
		return "delete"
	case DBDeleteNA:
		return "delete_na"
	}
	return "unknown"
}

const (
	DBNone DBAct = iota
	DBPut
	DBOverwrite
	DBDelete
	DBDeleteNA
)

type DBTesting struct {
	Rand *rand.Rand
	DB   interface {
		Get
		Put
		Delete
	}
	PostFn             func(t *DBTesting)
	Deleted, Present   KeyValue
	Act, LastAct       DBAct
	ActKey, LastActKey []byte
}

func (t *DBTesting) post() {
	if t.PostFn != nil {
		t.PostFn(t)
	}
}

func (t *DBTesting) setAct(act DBAct, key []byte) {
	t.LastAct, t.Act = t.Act, act
	t.LastActKey, t.ActKey = t.ActKey, key
}

func (t *DBTesting) text() string {
	return fmt.Sprintf("last action was <%v> %q, <%v> %q", t.LastAct, t.LastActKey, t.Act, t.ActKey)
}

func (t *DBTesting) Text() string {
	return "DBTesting " + t.text()
}

func (t *DBTesting) TestPresentKV(key, value []byte) {
	rvalue, err := t.DB.TestGet(key)
	Expect(err).ShouldNot(HaveOccurred(), "Get on key %q, %s", key, t.text())
	Expect(rvalue).Should(Equal(value), "Value for key %q, %s", key, t.text())
}

func (t *DBTesting) TestAllPresent() {
	t.Present.IterateShuffled(t.Rand, func(i int, key, value []byte) {
		t.TestPresentKV(key, value)
	})
}

func (t *DBTesting) TestDeletedKey(key []byte) {
	_, err := t.DB.TestGet(key)
	Expect(err).Should(Equal(errors.ErrNotFound), "Get on deleted key %q, %s", key, t.text())
}

func (t *DBTesting) TestAllDeleted() {
	t.Deleted.IterateShuffled(t.Rand, func(i int, key, value []byte) {
		t.TestDeletedKey(key)
	})
}

func (t *DBTesting) TestAll() {
	dn := t.Deleted.Len()
	pn := t.Present.Len()
	ShuffledIndex(t.Rand, dn+pn, 1, func(i int) {
		if i >= dn {
			key, value := t.Present.Index(i - dn)
			t.TestPresentKV(key, value)
		} else {
			t.TestDeletedKey(t.Deleted.KeyAt(i))
		}
	})
}

func (t *DBTesting) Put(key, value []byte) {
	if new := t.Present.PutU(key, value); new {
		t.setAct(DBPut, key)
	} else {
		t.setAct(DBOverwrite, key)
	}
	t.Deleted.Delete(key)
	err := t.DB.TestPut(key, value)
	Expect(err).ShouldNot(HaveOccurred(), t.Text())
	t.TestPresentKV(key, value)
	t.post()
}

func (t *DBTesting) PutRandom() bool {
	if t.Deleted.Len() > 0 {
		i := t.Rand.Intn(t.Deleted.Len())
		key, value := t.Deleted.Index(i)
		t.Put(key, value)
		return true
	}
	return false
}

func (t *DBTesting) Delete(key []byte) {
	if exist, value := t.Present.Delete(key); exist {
		t.setAct(DBDelete, key)
		t.Deleted.PutU(key, value)
	} else {
		t.setAct(DBDeleteNA, key)
	}
	err := t.DB.TestDelete(key)
	Expect(err).ShouldNot(HaveOccurred(), t.Text())
	t.TestDeletedKey(key)
	t.post()
}

func (t *DBTesting) DeleteRandom() bool {
	if t.Present.Len() > 0 {
		i := t.Rand.Intn(t.Present.Len())
		t.Delete(t.Present.KeyAt(i))
		return true
	}
	return false
}

func (t *DBTesting) RandomAct(round int) {
	for i := 0; i < round; i++ {
		if t.Rand.Int()%2 == 0 {
			t.PutRandom()
		} else {
			t.DeleteRandom()
		}
	}
}

func DoDBTesting(t *DBTesting) {
	if t.Rand == nil {
		t.Rand = NewRand()
	}

	t.DeleteRandom()
	t.PutRandom()
	t.DeleteRandom()
	t.DeleteRandom()
	for i := t.Deleted.Len() / 2; i >= 0; i-- {
		t.PutRandom()
	}
	t.RandomAct((t.Deleted.Len() + t.Present.Len()) * 10)

	// Additional iterator testing
	if db, ok := t.DB.(NewIterator); ok {
		iter := db.TestNewIterator(nil)
		Expect(iter.Error()).NotTo(HaveOccurred())

		it := IteratorTesting{
			KeyValue: t.Present,
			Iter:     iter,
		}

		DoIteratorTesting(&it)
		iter.Release()
	}
}
