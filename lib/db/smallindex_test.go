// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"testing"
)

func TestSmallIndex(t *testing.T) {
	db := newLowlevelMemory(t)
	idx := newSmallIndex(db, []byte{12, 34})

	// ID zero should be unallocated
	if val, ok := idx.Val(0); ok || val != nil {
		t.Fatal("Unexpected return for nonexistent ID 0")
	}

	// A new key should get ID zero
	if id, err := idx.ID([]byte("hello")); err != nil {
		t.Fatal(err)
	} else if id != 0 {
		t.Fatal("Expected 0, not", id)
	}
	// Looking up ID zero should work
	if val, ok := idx.Val(0); !ok || string(val) != "hello" {
		t.Fatalf(`Expected true, "hello", not %v, %q`, ok, val)
	}

	// Delete the key
	idx.Delete([]byte("hello"))

	// Next ID should be one
	if id, err := idx.ID([]byte("key2")); err != nil {
		t.Fatal(err)
	} else if id != 1 {
		t.Fatal("Expected 1, not", id)
	}

	// Now lets create a new index instance based on what's actually serialized to the database.
	idx = newSmallIndex(db, []byte{12, 34})

	// Status should be about the same as before.
	if val, ok := idx.Val(0); ok || val != nil {
		t.Fatal("Unexpected return for deleted ID 0")
	}
	if id, err := idx.ID([]byte("key2")); err != nil {
		t.Fatal(err)
	} else if id != 1 {
		t.Fatal("Expected 1, not", id)
	}

	// Setting "hello" again should get us ID 2, not 0 as it was originally.
	if id, err := idx.ID([]byte("hello")); err != nil {
		t.Fatal(err)
	} else if id != 2 {
		t.Fatal("Expected 2, not", id)
	}
}
