// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iterator_test

import (
	. "github.com/onsi/ginkgo"

	. "github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/testutil"
)

var _ = testutil.Defer(func() {
	Describe("Array iterator", func() {
		It("Should iterates and seeks correctly", func() {
			// Build key/value.
			kv := testutil.KeyValue_Generate(nil, 70, 1, 1, 5, 3, 3)

			// Test the iterator.
			t := testutil.IteratorTesting{
				KeyValue: kv.Clone(),
				Iter:     NewArrayIterator(kv),
			}
			testutil.DoIteratorTesting(&t)
		})
	})
})
