// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"testing"
	"time"
)

func TestNamespacedInt(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	n1 := NewNamespacedKV(ldb, "foo")
	n2 := NewNamespacedKV(ldb, "bar")

	// Key is missing to start with

	if v, ok, err := n1.Int64("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}

	if err := n1.PutInt64("test", 42); err != nil {
		t.Fatal(err)
	}

	// It should now exist in n1

	if v, ok, err := n1.Int64("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != 42 || !ok {
		t.Errorf("Incorrect return v %v != 42 || ok %v != true", v, ok)
	}

	// ... but not in n2, which is in a different namespace

	if v, ok, err := n2.Int64("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}

	if err := n1.Delete("test"); err != nil {
		t.Fatal(err)
	}

	// It should no longer exist

	if v, ok, err := n1.Int64("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}
}

func TestNamespacedTime(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	n1 := NewNamespacedKV(ldb, "foo")

	if v, ok, err := n1.Time("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if !v.IsZero() || ok {
		t.Errorf("Incorrect return v %v != %v || ok %v != false", v, time.Time{}, ok)
	}

	now := time.Now()
	if err := n1.PutTime("test", now); err != nil {
		t.Fatal(err)
	}

	if v, ok, err := n1.Time("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if !v.Equal(now) || !ok {
		t.Errorf("Incorrect return v %v != %v || ok %v != true", v, now, ok)
	}
}

func TestNamespacedString(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	n1 := NewNamespacedKV(ldb, "foo")

	if v, ok, err := n1.String("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}

	if err := n1.PutString("test", "yo"); err != nil {
		t.Fatal(err)
	}

	if v, ok, err := n1.String("test"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "yo" || !ok {
		t.Errorf("Incorrect return v %q != \"yo\" || ok %v != true", v, ok)
	}
}

func TestNamespacedReset(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	n1 := NewNamespacedKV(ldb, "foo")

	if err := n1.PutString("test1", "yo1"); err != nil {
		t.Fatal(err)
	}
	if err := n1.PutString("test2", "yo2"); err != nil {
		t.Fatal(err)
	}
	if err := n1.PutString("test3", "yo3"); err != nil {
		t.Fatal(err)
	}

	if v, ok, err := n1.String("test1"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "yo1" || !ok {
		t.Errorf("Incorrect return v %q != \"yo1\" || ok %v != true", v, ok)
	}
	if v, ok, err := n1.String("test2"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "yo2" || !ok {
		t.Errorf("Incorrect return v %q != \"yo2\" || ok %v != true", v, ok)
	}
	if v, ok, err := n1.String("test3"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "yo3" || !ok {
		t.Errorf("Incorrect return v %q != \"yo3\" || ok %v != true", v, ok)
	}

	reset(n1)

	if v, ok, err := n1.String("test1"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}
	if v, ok, err := n1.String("test2"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}
	if v, ok, err := n1.String("test3"); err != nil {
		t.Error("Unexpected error:", err)
	} else if v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}
}

// reset removes all entries in this namespace.
func reset(n *NamespacedKV) {
	tr, err := n.db.NewWriteTransaction()
	if err != nil {
		return
	}
	defer tr.Release()

	it, err := tr.NewPrefixIterator([]byte(n.prefix))
	if err != nil {
		return
	}
	for it.Next() {
		_ = tr.Delete(it.Key())
	}
	it.Release()
	_ = tr.Commit()
}
