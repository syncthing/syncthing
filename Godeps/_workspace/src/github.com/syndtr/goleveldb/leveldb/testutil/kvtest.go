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

	"github.com/syndtr/goleveldb/leveldb/util"
)

func KeyValueTesting(rnd *rand.Rand, p DB, kv KeyValue) {
	if rnd == nil {
		rnd = NewRand()
	}

	if db, ok := p.(Find); ok {
		It("Should find all keys with Find", func() {
			ShuffledIndex(nil, kv.Len(), 1, func(i int) {
				key_, key, value := kv.IndexInexact(i)

				// Using exact key.
				rkey, rvalue, err := db.TestFind(key)
				Expect(err).ShouldNot(HaveOccurred(), "Error for key %q", key)
				Expect(rkey).Should(Equal(key), "Key")
				Expect(rvalue).Should(Equal(value), "Value for key %q", key)

				// Using inexact key.
				rkey, rvalue, err = db.TestFind(key_)
				Expect(err).ShouldNot(HaveOccurred(), "Error for key %q (%q)", key_, key)
				Expect(rkey).Should(Equal(key))
				Expect(rvalue).Should(Equal(value), "Value for key %q (%q)", key_, key)
			})
		})

		It("Should return error if the key is not present", func() {
			var key []byte
			if kv.Len() > 0 {
				key_, _ := kv.Index(kv.Len() - 1)
				key = BytesAfter(key_)
			}
			rkey, _, err := db.TestFind(key)
			Expect(err).Should(HaveOccurred(), "Find for key %q yield key %q", key, rkey)
			Expect(err).Should(Equal(util.ErrNotFound))
		})
	}

	if db, ok := p.(Get); ok {
		It("Should only find exact key with Get", func() {
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
					Expect(err).Should(Equal(util.ErrNotFound))
				}
			})
		})
	}

	if db, ok := p.(NewIterator); ok {
		TestIter := func(r *util.Range, _kv KeyValue) {
			iter := db.TestNewIterator(r)
			Expect(iter.Error()).ShouldNot(HaveOccurred())

			t := IteratorTesting{
				KeyValue: _kv,
				Iter:     iter,
			}

			DoIteratorTesting(&t)
		}

		It("Should iterates and seeks correctly", func(done Done) {
			TestIter(nil, kv.Clone())
			done <- true
		}, 3.0)

		RandomIndex(rnd, kv.Len(), kv.Len(), func(i int) {
			type slice struct {
				r            *util.Range
				start, limit int
			}

			key_, _, _ := kv.IndexInexact(i)
			for _, x := range []slice{
				{&util.Range{Start: key_, Limit: nil}, i, kv.Len()},
				{&util.Range{Start: nil, Limit: key_}, 0, i},
			} {
				It(fmt.Sprintf("Should iterates and seeks correctly of a slice %d .. %d", x.start, x.limit), func(done Done) {
					TestIter(x.r, kv.Slice(x.start, x.limit))
					done <- true
				}, 3.0)
			}
		})

		RandomRange(rnd, kv.Len(), kv.Len(), func(start, limit int) {
			It(fmt.Sprintf("Should iterates and seeks correctly of a slice %d .. %d", start, limit), func(done Done) {
				r := kv.Range(start, limit)
				TestIter(&r, kv.Slice(start, limit))
				done <- true
			}, 3.0)
		})
	}
}

func AllKeyValueTesting(rnd *rand.Rand, body func(kv KeyValue) DB) {
	Test := func(kv *KeyValue) func() {
		return func() {
			db := body(*kv)
			KeyValueTesting(rnd, db, *kv)
		}
	}

	Describe("with no key/value (empty)", Test(&KeyValue{}))
	Describe("with empty key", Test(KeyValue_EmptyKey()))
	Describe("with empty value", Test(KeyValue_EmptyValue()))
	Describe("with one key/value", Test(KeyValue_OneKeyValue()))
	Describe("with big value", Test(KeyValue_BigValue()))
	Describe("with special key", Test(KeyValue_SpecialKey()))
	Describe("with multiple key/value", Test(KeyValue_MultipleKeyValue()))
}
