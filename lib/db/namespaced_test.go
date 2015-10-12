// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func TestNamespacedInt(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")
	n2 := NewNamespacedKV(ldb, "bar")

	// Key is missing to start with

	if v, ok := n1.Int64("test"); v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}

	n1.PutInt64("test", 42)

	// It should now exist in n1

	if v, ok := n1.Int64("test"); v != 42 || !ok {
		t.Errorf("Incorrect return v %v != 42 || ok %v != true", v, ok)
	}

	// ... but not in n2, which is in a different namespace

	if v, ok := n2.Int64("test"); v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}

	n1.Delete("test")

	// It should no longer exist

	if v, ok := n1.Int64("test"); v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}
}

func TestNamespacedTime(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")

	if v, ok := n1.Time("test"); v != (time.Time{}) || ok {
		t.Errorf("Incorrect return v %v != %v || ok %v != false", v, time.Time{}, ok)
	}

	now := time.Now()
	n1.PutTime("test", now)

	if v, ok := n1.Time("test"); v != now || !ok {
		t.Errorf("Incorrect return v %v != %v || ok %v != true", v, now, ok)
	}
}

func TestNamespacedString(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")

	if v, ok := n1.String("test"); v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}

	n1.PutString("test", "yo")

	if v, ok := n1.String("test"); v != "yo" || !ok {
		t.Errorf("Incorrect return v %q != \"yo\" || ok %v != true", v, ok)
	}
}

func TestNamespacedReset(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")

	n1.PutString("test1", "yo1")
	n1.PutString("test2", "yo2")
	n1.PutString("test3", "yo3")

	if v, ok := n1.String("test1"); v != "yo1" || !ok {
		t.Errorf("Incorrect return v %q != \"yo1\" || ok %v != true", v, ok)
	}
	if v, ok := n1.String("test2"); v != "yo2" || !ok {
		t.Errorf("Incorrect return v %q != \"yo2\" || ok %v != true", v, ok)
	}
	if v, ok := n1.String("test3"); v != "yo3" || !ok {
		t.Errorf("Incorrect return v %q != \"yo3\" || ok %v != true", v, ok)
	}

	n1.Reset()

	if v, ok := n1.String("test1"); v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}
	if v, ok := n1.String("test2"); v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}
	if v, ok := n1.String("test3"); v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}
}
