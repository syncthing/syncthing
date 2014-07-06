// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package table

import (
	"encoding/binary"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/testutil"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func (b *block) TestNewIterator(slice *util.Range) iterator.Iterator {
	return b.newIterator(slice, false, nil)
}

var _ = testutil.Defer(func() {
	Describe("Block", func() {
		Build := func(kv *testutil.KeyValue, restartInterval int) *block {
			// Building the block.
			bw := &blockWriter{
				restartInterval: restartInterval,
				scratch:         make([]byte, 30),
			}
			kv.Iterate(func(i int, key, value []byte) {
				bw.append(key, value)
			})
			bw.finish()

			// Opening the block.
			data := bw.buf.Bytes()
			restartsLen := int(binary.LittleEndian.Uint32(data[len(data)-4:]))
			return &block{
				cmp:            comparer.DefaultComparer,
				data:           data,
				restartsLen:    restartsLen,
				restartsOffset: len(data) - (restartsLen+1)*4,
			}
		}

		Describe("read test", func() {
			for restartInterval := 1; restartInterval <= 5; restartInterval++ {
				Describe(fmt.Sprintf("with restart interval of %d", restartInterval), func() {
					kv := &testutil.KeyValue{}
					Text := func() string {
						return fmt.Sprintf("and %d keys", kv.Len())
					}

					Test := func() {
						// Make block.
						br := Build(kv, restartInterval)
						// Do testing.
						testutil.KeyValueTesting(nil, br, kv.Clone())
					}

					Describe(Text(), Test)

					kv.PutString("", "empty")
					Describe(Text(), Test)

					kv.PutString("a1", "foo")
					Describe(Text(), Test)

					kv.PutString("a2", "v")
					Describe(Text(), Test)

					kv.PutString("a3qqwrkks", "hello")
					Describe(Text(), Test)

					kv.PutString("a4", "bar")
					Describe(Text(), Test)

					kv.PutString("a5111111", "v5")
					kv.PutString("a6", "")
					kv.PutString("a7", "v7")
					kv.PutString("a8", "vvvvvvvvvvvvvvvvvvvvvv8")
					kv.PutString("b", "v9")
					kv.PutString("c9", "v9")
					kv.PutString("c91", "v9")
					kv.PutString("d0", "v9")
					Describe(Text(), Test)
				})
			}
		})

		Describe("out-of-bound slice test", func() {
			kv := &testutil.KeyValue{}
			kv.PutString("k1", "v1")
			kv.PutString("k2", "v2")
			kv.PutString("k3abcdefgg", "v3")
			kv.PutString("k4", "v4")
			kv.PutString("k5", "v5")
			for restartInterval := 1; restartInterval <= 5; restartInterval++ {
				Describe(fmt.Sprintf("with restart interval of %d", restartInterval), func() {
					// Make block.
					br := Build(kv, restartInterval)

					Test := func(r *util.Range) func(done Done) {
						return func(done Done) {
							iter := br.newIterator(r, false, nil)
							Expect(iter.Error()).ShouldNot(HaveOccurred())

							t := testutil.IteratorTesting{
								KeyValue: kv.Clone(),
								Iter:     iter,
							}

							testutil.DoIteratorTesting(&t)
							done <- true
						}
					}

					It("Should do iterations and seeks correctly #0",
						Test(&util.Range{Start: []byte("k0"), Limit: []byte("k6")}), 2.0)

					It("Should do iterations and seeks correctly #1",
						Test(&util.Range{Start: []byte(""), Limit: []byte("zzzzzzz")}), 2.0)
				})
			}
		})
	})
})
