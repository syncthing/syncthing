// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testutil

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func TestFind(db Find, kv KeyValue) {
	ShuffledIndex(nil, kv.Len(), 1, func(i int) {
		key_, key, value := kv.IndexInexact(i)

		// Using exact key.
		rkey, rvalue, err := db.TestFind(key)
		Expect(err).ShouldNot(HaveOccurred(), "Error for exact key %q", key)
		Expect(rkey).Should(Equal(key), "Key")
		Expect(rvalue).Should(Equal(value), "Value for exact key %q", key)

		// Using inexact key.
		rkey, rvalue, err = db.TestFind(key_)
		Expect(err).ShouldNot(HaveOccurred(), "Error for inexact key %q (%q)", key_, key)
		Expect(rkey).Should(Equal(key), "Key for inexact key %q (%q)", key_, key)
		Expect(rvalue).Should(Equal(value), "Value for inexact key %q (%q)", key_, key)
	})
}

func TestFindAfterLast(db Find, kv KeyValue) {
	var key []byte
	if kv.Len() > 0 {
		key_, _ := kv.Index(kv.Len() - 1)
		key = BytesAfter(key_)
	}
	rkey, _, err := db.TestFind(key)
	Expect(err).Should(HaveOccurred(), "Find for key %q yield key %q", key, rkey)
	Expect(err).Should(Equal(errors.ErrNotFound))
}

func TestGet(db Get, kv KeyValue) {
	ShuffledIndex(nil, kv.Len(), 1, func(i int) {
		key_, key, value := kv.IndexInexact(i)

		// Using exact key.
		rvalue, err := db.TestGet(key)
		Expect(err).ShouldNot(HaveOccurred(), "Error for key %q", key)
		Expect(rvalue).Should(Equal(value), "Value for key %q", key)

		// Using inexact key.
		if len(key_) > 0 {
			_, err = db.TestGet(key_)
			Expect(err).Should(HaveOccurred(), "Error for key %q", key_)
			Expect(err).Should(Equal(errors.ErrNotFound))
		}
	})
}

func TestHas(db Has, kv KeyValue) {
	ShuffledIndex(nil, kv.Len(), 1, func(i int) {
		key_, key, _ := kv.IndexInexact(i)

		// Using exact key.
		ret, err := db.TestHas(key)
		Expect(err).ShouldNot(HaveOccurred(), "Error for key %q", key)
		Expect(ret).Should(BeTrue(), "False for key %q", key)

		// Using inexact key.
		if len(key_) > 0 {
			ret, err = db.TestHas(key_)
			Expect(err).ShouldNot(HaveOccurred(), "Error for key %q", key_)
			Expect(ret).ShouldNot(BeTrue(), "True for key %q", key)
		}
	})
}

func TestIter(db NewIterator, r *util.Range, kv KeyValue) {
	iter := db.TestNewIterator(r)
	Expect(iter.Error()).ShouldNot(HaveOccurred())

	t := IteratorTesting{
		KeyValue: kv,
		Iter:     iter,
	}

	DoIteratorTesting(&t)
	iter.Release()
}

func KeyValueTesting(rnd *rand.Rand, kv KeyValue, p DB, setup func(KeyValue) DB, teardown func(DB)) {
	if rnd == nil {
		rnd = NewRand()
	}

	if p == nil {
		BeforeEach(func() {
			p = setup(kv)
		})
		if teardown != nil {
			AfterEach(func() {
				teardown(p)
			})
		}
	}

	It("Should find all keys with Find", func() {
		if db, ok := p.(Find); ok {
			TestFind(db, kv)
		}
	})

	It("Should return error if Find on key after the last", func() {
		if db, ok := p.(Find); ok {
			TestFindAfterLast(db, kv)
		}
	})

	It("Should only find exact key with Get", func() {
		if db, ok := p.(Get); ok {
			TestGet(db, kv)
		}
	})

	It("Should only find present key with Has", func() {
		if db, ok := p.(Has); ok {
			TestHas(db, kv)
		}
	})

	It("Should iterates and seeks correctly", func(done Done) {
		if db, ok := p.(NewIterator); ok {
			TestIter(db, nil, kv.Clone())
		}
		done <- true
	}, 30.0)

	It("Should iterates and seeks slice correctly", func(done Done) {
		if db, ok := p.(NewIterator); ok {
			RandomIndex(rnd, kv.Len(), Min(kv.Len(), 50), func(i int) {
				type slice struct {
					r            *util.Range
					start, limit int
				}

				key_, _, _ := kv.IndexInexact(i)
				for _, x := range []slice{
					{&util.Range{Start: key_, Limit: nil}, i, kv.Len()},
					{&util.Range{Start: nil, Limit: key_}, 0, i},
				} {
					By(fmt.Sprintf("Random index of %d .. %d", x.start, x.limit), func() {
						TestIter(db, x.r, kv.Slice(x.start, x.limit))
					})
				}
			})
		}
		done <- true
	}, 200.0)

	It("Should iterates and seeks slice correctly", func(done Done) {
		if db, ok := p.(NewIterator); ok {
			RandomRange(rnd, kv.Len(), Min(kv.Len(), 50), func(start, limit int) {
				By(fmt.Sprintf("Random range of %d .. %d", start, limit), func() {
					r := kv.Range(start, limit)
					TestIter(db, &r, kv.Slice(start, limit))
				})
			})
		}
		done <- true
	}, 200.0)
}

func AllKeyValueTesting(rnd *rand.Rand, body, setup func(KeyValue) DB, teardown func(DB)) {
	Test := func(kv *KeyValue) func() {
		return func() {
			var p DB
			if setup != nil {
				Defer("setup", func() {
					p = setup(*kv)
				})
			}
			if teardown != nil {
				Defer("teardown", func() {
					teardown(p)
				})
			}
			if body != nil {
				p = body(*kv)
			}
			KeyValueTesting(rnd, *kv, p, func(KeyValue) DB {
				return p
			}, nil)
		}
	}

	Describe("with no key/value (empty)", Test(&KeyValue{}))
	Describe("with empty key", Test(KeyValue_EmptyKey()))
	Describe("with empty value", Test(KeyValue_EmptyValue()))
	Describe("with one key/value", Test(KeyValue_OneKeyValue()))
	Describe("with big value", Test(KeyValue_BigValue()))
	Describe("with special key", Test(KeyValue_SpecialKey()))
	Describe("with multiple key/value", Test(KeyValue_MultipleKeyValue()))
	Describe("with generated key/value 2-incr", Test(KeyValue_Generate(nil, 120, 2, 1, 50, 10, 120)))
	Describe("with generated key/value 3-incr", Test(KeyValue_Generate(nil, 120, 3, 1, 50, 10, 120)))
}
