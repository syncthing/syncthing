// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import "testing"

// testBackendBehavior is the generic test suite that must be fulfilled by
// every backend implementation. It should be called by each implementation
// as (part of) their test suite.
func testBackendBehavior(t *testing.T, open func() (Backend, error)) {
	t.Run("WriteIsolation", func(t *testing.T) { testWriteIsolation(t, open) })
	t.Run("DeleteNonexisten", func(t *testing.T) { testDeleteNonexistent(t, open) })
	t.Run("IteratorClosedDB", func(t *testing.T) { testIteratorClosedDB(t, open) })
}

func testWriteIsolation(t *testing.T, open func() (Backend, error)) {
	// Values written during a transaction should not be read back, our
	// updateGlobal depends on this.

	db, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Sanity check
	_ = db.Put([]byte("a"), []byte("a"))
	v, _ := db.Get([]byte("a"))
	if string(v) != "a" {
		t.Fatal("read back should work")
	}

	// Now in a transaction we should still see the old value
	tx, _ := db.NewWriteTransaction()
	defer tx.Release()
	_ = tx.Put([]byte("a"), []byte("b"))
	v, _ = tx.Get([]byte("a"))
	if string(v) != "a" {
		t.Fatal("read in transaction should read the old value")
	}
}

func testDeleteNonexistent(t *testing.T, open func() (Backend, error)) {
	// Deleting a non-existent key is not an error

	db, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.Delete([]byte("a"))
	if err != nil {
		t.Error(err)
	}
}

// Either creating the iterator or the .Error() method of the returned iterator
// should return an error and IsClosed(err) == true.
func testIteratorClosedDB(t *testing.T, open func() (Backend, error)) {
	db, err := open()
	if err != nil {
		t.Fatal(err)
	}

	_ = db.Put([]byte("a"), []byte("a"))

	db.Close()

	it, err := db.NewPrefixIterator(nil)
	if err != nil {
		if !IsClosed(err) {
			t.Error("NewPrefixIterator: IsClosed(err) == false:", err)
		}
		return
	}
	it.Next()
	if err := it.Error(); !IsClosed(err) {
		t.Error("Next: IsClosed(err) == false:", err)
	}
}
