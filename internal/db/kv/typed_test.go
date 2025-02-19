// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package kv_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db/kv"
	"github.com/syncthing/syncthing/internal/db/sqlite"
)

func TestNamespacedInt(t *testing.T) {
	ldb, err := sqlite.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ldb.Close()

	n1 := kv.NewTyped(ldb, "foo")
	n2 := kv.NewTyped(ldb, "bar")

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
	ldb, err := sqlite.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ldb.Close()

	n1 := kv.NewTyped(ldb, "foo")

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
	ldb, err := sqlite.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ldb.Close()

	n1 := kv.NewTyped(ldb, "foo")

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
