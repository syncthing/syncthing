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
	})
})
