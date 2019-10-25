// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import "testing"

func TestLevelDBWriteIsolation(t *testing.T) {
	// Values written during a transaction should not be read back, our
	// updateGlobal depends on this.

	db := OpenLevelDBMemory()
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

func TestLevelDBDeleteNonexistent(t *testing.T) {
	// Deleting a non-existent key is not an error

	db := OpenLevelDBMemory()
	defer db.Close()

	err := db.Delete([]byte("a"))
	if err != nil {
		t.Error(err)
	}
}
