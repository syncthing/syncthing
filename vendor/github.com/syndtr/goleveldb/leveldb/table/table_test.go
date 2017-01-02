// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package table

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/testutil"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type tableWrapper struct {
	*Reader
}

func (t tableWrapper) TestFind(key []byte) (rkey, rvalue []byte, err error) {
	return t.Reader.Find(key, false, nil)
}

func (t tableWrapper) TestGet(key []byte) (value []byte, err error) {
	return t.Reader.Get(key, nil)
}

func (t tableWrapper) TestNewIterator(slice *util.Range) iterator.Iterator {
	return t.Reader.NewIterator(slice, nil)
}

var _ = testutil.Defer(func() {
	Describe("Table", func() {
		Describe("approximate offset test", func() {
			var (
				buf = &bytes.Buffer{}
				o   = &opt.Options{
					BlockSize:   1024,
					Compression: opt.NoCompression,
				}
			)

			// Building the table.
			tw := NewWriter(buf, o)
			tw.Append([]byte("k01"), []byte("hello"))
			tw.Append([]byte("k02"), []byte("hello2"))
			tw.Append([]byte("k03"), bytes.Repeat([]byte{'x'}, 10000))
			tw.Append([]byte("k04"), bytes.Repeat([]byte{'x'}, 200000))
			tw.Append([]byte("k05"), bytes.Repeat([]byte{'x'}, 300000))
			tw.Append([]byte("k06"), []byte("hello3"))
			tw.Append([]byte("k07"), bytes.Repeat([]byte{'x'}, 100000))
			err := tw.Close()

			It("Should be able to approximate offset of a key correctly", func() {
				Expect(err).ShouldNot(HaveOccurred())

				tr, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), storage.FileDesc{}, nil, nil, o)
				Expect(err).ShouldNot(HaveOccurred())
				CheckOffset := func(key string, expect, threshold int) {
					offset, err := tr.OffsetOf([]byte(key))
					Expect(err).ShouldNot(HaveOccurred())
					Expect(offset).Should(BeNumerically("~", expect, threshold), "Offset of key %q", key)
				}

				CheckOffset("k0", 0, 0)
				CheckOffset("k01a", 0, 0)
				CheckOffset("k02", 0, 0)
				CheckOffset("k03", 0, 0)
				CheckOffset("k04", 10000, 1000)
				CheckOffset("k04a", 210000, 1000)
				CheckOffset("k05", 210000, 1000)
				CheckOffset("k06", 510000, 1000)
				CheckOffset("k07", 510000, 1000)
				CheckOffset("xyz", 610000, 2000)
			})
		})

		Describe("read test", func() {
			Build := func(kv testutil.KeyValue) testutil.DB {
				o := &opt.Options{
					BlockSize:            512,
					BlockRestartInterval: 3,
				}
				buf := &bytes.Buffer{}

				// Building the table.
				tw := NewWriter(buf, o)
				kv.Iterate(func(i int, key, value []byte) {
					tw.Append(key, value)
				})
				tw.Close()

				// Opening the table.
				tr, _ := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), storage.FileDesc{}, nil, nil, o)
				return tableWrapper{tr}
			}
			Test := func(kv *testutil.KeyValue, body func(r *Reader)) func() {
				return func() {
					db := Build(*kv)
					if body != nil {
						body(db.(tableWrapper).Reader)
					}
					testutil.KeyValueTesting(nil, *kv, db, nil, nil)
				}
			}

			testutil.AllKeyValueTesting(nil, Build, nil, nil)
			Describe("with one key per block", Test(testutil.KeyValue_Generate(nil, 9, 1, 1, 10, 512, 512), func(r *Reader) {
				It("should have correct blocks number", func() {
					indexBlock, err := r.readBlock(r.indexBH, true)
					Expect(err).To(BeNil())
					Expect(indexBlock.restartsLen).Should(Equal(9))
				})
			}))
		})
	})
})
