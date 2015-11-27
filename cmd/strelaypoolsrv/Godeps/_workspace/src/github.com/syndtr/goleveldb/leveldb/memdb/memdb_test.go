// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memdb

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/testutil"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func (p *DB) TestFindLT(key []byte) (rkey, value []byte, err error) {
	p.mu.RLock()
	if node := p.findLT(key); node != 0 {
		n := p.nodeData[node]
		m := n + p.nodeData[node+nKey]
		rkey = p.kvData[n:m]
		value = p.kvData[m : m+p.nodeData[node+nVal]]
	} else {
		err = ErrNotFound
	}
	p.mu.RUnlock()
	return
}

func (p *DB) TestFindLast() (rkey, value []byte, err error) {
	p.mu.RLock()
	if node := p.findLast(); node != 0 {
		n := p.nodeData[node]
		m := n + p.nodeData[node+nKey]
		rkey = p.kvData[n:m]
		value = p.kvData[m : m+p.nodeData[node+nVal]]
	} else {
		err = ErrNotFound
	}
	p.mu.RUnlock()
	return
}

func (p *DB) TestPut(key []byte, value []byte) error {
	p.Put(key, value)
	return nil
}

func (p *DB) TestDelete(key []byte) error {
	p.Delete(key)
	return nil
}

func (p *DB) TestFind(key []byte) (rkey, rvalue []byte, err error) {
	return p.Find(key)
}

func (p *DB) TestGet(key []byte) (value []byte, err error) {
	return p.Get(key)
}

func (p *DB) TestNewIterator(slice *util.Range) iterator.Iterator {
	return p.NewIterator(slice)
}

var _ = testutil.Defer(func() {
	Describe("Memdb", func() {
		Describe("write test", func() {
			It("should do write correctly", func() {
				db := New(comparer.DefaultComparer, 0)
				t := testutil.DBTesting{
					DB:      db,
					Deleted: testutil.KeyValue_Generate(nil, 1000, 1, 30, 5, 5).Clone(),
					PostFn: func(t *testutil.DBTesting) {
						Expect(db.Len()).Should(Equal(t.Present.Len()))
						Expect(db.Size()).Should(Equal(t.Present.Size()))
						switch t.Act {
						case testutil.DBPut, testutil.DBOverwrite:
							Expect(db.Contains(t.ActKey)).Should(BeTrue())
						default:
							Expect(db.Contains(t.ActKey)).Should(BeFalse())
						}
					},
				}
				testutil.DoDBTesting(&t)
			})
		})

		Describe("read test", func() {
			testutil.AllKeyValueTesting(nil, func(kv testutil.KeyValue) testutil.DB {
				// Building the DB.
				db := New(comparer.DefaultComparer, 0)
				kv.IterateShuffled(nil, func(i int, key, value []byte) {
					db.Put(key, value)
				})

				if kv.Len() > 1 {
					It("Should find correct keys with findLT", func() {
						testutil.ShuffledIndex(nil, kv.Len()-1, 1, func(i int) {
							key_, key, _ := kv.IndexInexact(i + 1)
							expectedKey, expectedValue := kv.Index(i)

							// Using key that exist.
							rkey, rvalue, err := db.TestFindLT(key)
							Expect(err).ShouldNot(HaveOccurred(), "Error for key %q -> %q", key, expectedKey)
							Expect(rkey).Should(Equal(expectedKey), "Key")
							Expect(rvalue).Should(Equal(expectedValue), "Value for key %q -> %q", key, expectedKey)

							// Using key that doesn't exist.
							rkey, rvalue, err = db.TestFindLT(key_)
							Expect(err).ShouldNot(HaveOccurred(), "Error for key %q (%q) -> %q", key_, key, expectedKey)
							Expect(rkey).Should(Equal(expectedKey))
							Expect(rvalue).Should(Equal(expectedValue), "Value for key %q (%q) -> %q", key_, key, expectedKey)
						})
					})
				}

				if kv.Len() > 0 {
					It("Should find last key with findLast", func() {
						key, value := kv.Index(kv.Len() - 1)
						rkey, rvalue, err := db.TestFindLast()
						Expect(err).ShouldNot(HaveOccurred())
						Expect(rkey).Should(Equal(key))
						Expect(rvalue).Should(Equal(value))
					})
				}

				return db
			}, nil, nil)
		})
	})
})
