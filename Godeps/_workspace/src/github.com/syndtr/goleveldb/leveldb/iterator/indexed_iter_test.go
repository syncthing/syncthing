// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iterator_test

import (
	"sort"

	. "github.com/onsi/ginkgo"

	"github.com/syndtr/goleveldb/leveldb/comparer"
	. "github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/testutil"
)

type keyValue struct {
	key []byte
	testutil.KeyValue
}

type keyValueIndex []keyValue

func (x keyValueIndex) Search(key []byte) int {
	return sort.Search(x.Len(), func(i int) bool {
		return comparer.DefaultComparer.Compare(x[i].key, key) >= 0
	})
}

func (x keyValueIndex) Len() int                        { return len(x) }
func (x keyValueIndex) Index(i int) (key, value []byte) { return x[i].key, nil }
func (x keyValueIndex) Get(i int) Iterator              { return NewArrayIterator(x[i]) }

var _ = testutil.Defer(func() {
	Describe("Indexed iterator", func() {
		Test := func(n ...int) func() {
			if len(n) == 0 {
				rnd := testutil.NewRand()
				n = make([]int, rnd.Intn(17)+3)
				for i := range n {
					n[i] = rnd.Intn(19) + 1
				}
			}

			return func() {
				It("Should iterates and seeks correctly", func(done Done) {
					// Build key/value.
					index := make(keyValueIndex, len(n))
					sum := 0
					for _, x := range n {
						sum += x
					}
					kv := testutil.KeyValue_Generate(nil, sum, 1, 10, 4, 4)
					for i, j := 0, 0; i < len(n); i++ {
						for x := n[i]; x > 0; x-- {
							key, value := kv.Index(j)
							index[i].key = key
							index[i].Put(key, value)
							j++
						}
					}

					// Test the iterator.
					t := testutil.IteratorTesting{
						KeyValue: kv.Clone(),
						Iter:     NewIndexedIterator(NewArrayIndexer(index), true),
					}
					testutil.DoIteratorTesting(&t)
					done <- true
				}, 1.5)
			}
		}

		Describe("with 100 keys", Test(100))
		Describe("with 50-50 keys", Test(50, 50))
		Describe("with 50-1 keys", Test(50, 1))
		Describe("with 50-1-50 keys", Test(50, 1, 50))
		Describe("with 1-50 keys", Test(1, 50))
		Describe("with random N-keys", Test())
	})
})
