// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/testutil"
)

var _ = testutil.Defer(func() {
	Describe("Leveldb external", func() {
		o := &opt.Options{
			DisableBlockCache:      true,
			BlockRestartInterval:   5,
			BlockSize:              80,
			Compression:            opt.NoCompression,
			OpenFilesCacheCapacity: -1,
			Strict:                 opt.StrictAll,
			WriteBuffer:            1000,
			CompactionTableSize:    2000,
		}

		Describe("write test", func() {
			It("should do write correctly", func(done Done) {
				db := newTestingDB(o, nil, nil)
				t := testutil.DBTesting{
					DB:      db,
					Deleted: testutil.KeyValue_Generate(nil, 500, 1, 50, 5, 5).Clone(),
				}
				testutil.DoDBTesting(&t)
				db.TestClose()
				done <- true
			}, 20.0)
		})

		Describe("read test", func() {
			testutil.AllKeyValueTesting(nil, nil, func(kv testutil.KeyValue) testutil.DB {
				// Building the DB.
				db := newTestingDB(o, nil, nil)
				kv.IterateShuffled(nil, func(i int, key, value []byte) {
					err := db.TestPut(key, value)
					Expect(err).NotTo(HaveOccurred())
				})

				return db
			}, func(db testutil.DB) {
				db.(*testingDB).TestClose()
			})
		})

		Describe("transaction test", func() {
			It("should do transaction correctly", func(done Done) {
				db := newTestingDB(o, nil, nil)

				By("creating first transaction")
				var err error
				tr := &testingTransaction{}
				tr.Transaction, err = db.OpenTransaction()
				Expect(err).NotTo(HaveOccurred())
				t0 := &testutil.DBTesting{
					DB:      tr,
					Deleted: testutil.KeyValue_Generate(nil, 200, 1, 50, 5, 5).Clone(),
				}
				testutil.DoDBTesting(t0)
				testutil.TestGet(tr, t0.Present)
				testutil.TestHas(tr, t0.Present)

				By("committing first transaction")
				err = tr.Commit()
				Expect(err).NotTo(HaveOccurred())
				testutil.TestIter(db, nil, t0.Present)
				testutil.TestGet(db, t0.Present)
				testutil.TestHas(db, t0.Present)

				By("manipulating DB without transaction")
				t0.DB = db
				testutil.DoDBTesting(t0)

				By("creating second transaction")
				tr.Transaction, err = db.OpenTransaction()
				Expect(err).NotTo(HaveOccurred())
				t1 := &testutil.DBTesting{
					DB:      tr,
					Deleted: t0.Deleted.Clone(),
					Present: t0.Present.Clone(),
				}
				testutil.DoDBTesting(t1)
				testutil.TestIter(db, nil, t0.Present)

				By("discarding second transaction")
				tr.Discard()
				testutil.TestIter(db, nil, t0.Present)

				By("creating third transaction")
				tr.Transaction, err = db.OpenTransaction()
				Expect(err).NotTo(HaveOccurred())
				t0.DB = tr
				testutil.DoDBTesting(t0)

				By("committing third transaction")
				err = tr.Commit()
				Expect(err).NotTo(HaveOccurred())
				testutil.TestIter(db, nil, t0.Present)

				db.TestClose()
				done <- true
			}, 30.0)
		})
	})
})
