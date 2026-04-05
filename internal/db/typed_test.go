// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db_test

import (
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/sqlite"
)

func TestNamespacedInt(t *testing.T) {
	t.Parallel()

	ldb, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ldb.Close()
	})

	n1 := db.NewTyped(ldb, "foo")
	n2 := db.NewTyped(ldb, "bar")

	t.Run("Int", func(t *testing.T) {
		t.Parallel()

		// Key is missing to start with

		if v, ok, err := n1.Int64("testint"); err != nil {
			t.Error("Unexpected error:", err)
		} else if v != 0 || ok {
			t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
		}

		if err := n1.PutInt64("testint", 42); err != nil {
			t.Fatal(err)
		}

		// It should now exist in n1

		if v, ok, err := n1.Int64("testint"); err != nil {
			t.Error("Unexpected error:", err)
		} else if v != 42 || !ok {
			t.Errorf("Incorrect return v %v != 42 || ok %v != true", v, ok)
		}

		// ... but not in n2, which is in a different namespace

		if v, ok, err := n2.Int64("testint"); err != nil {
			t.Error("Unexpected error:", err)
		} else if v != 0 || ok {
			t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
		}

		if err := n1.Delete("testint"); err != nil {
			t.Fatal(err)
		}

		// It should no longer exist

		if v, ok, err := n1.Int64("testint"); err != nil {
			t.Error("Unexpected error:", err)
		} else if v != 0 || ok {
			t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
		}
	})

	t.Run("Time", func(t *testing.T) {
		t.Parallel()

		if v, ok, err := n1.Time("testtime"); err != nil {
			t.Error("Unexpected error:", err)
		} else if !v.IsZero() || ok {
			t.Errorf("Incorrect return v %v != %v || ok %v != false", v, time.Time{}, ok)
		}

		now := time.Now()
		if err := n1.PutTime("testtime", now); err != nil {
			t.Fatal(err)
		}

		if v, ok, err := n1.Time("testtime"); err != nil {
			t.Error("Unexpected error:", err)
		} else if !v.Equal(now) || !ok {
			t.Errorf("Incorrect return v %v != %v || ok %v != true", v, now, ok)
		}
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()

		if v, ok, err := n1.String("teststring"); err != nil {
			t.Error("Unexpected error:", err)
		} else if v != "" || ok {
			t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
		}

		if err := n1.PutString("teststring", "yo"); err != nil {
			t.Fatal(err)
		}

		if v, ok, err := n1.String("teststring"); err != nil {
			t.Error("Unexpected error:", err)
		} else if v != "yo" || !ok {
			t.Errorf("Incorrect return v %q != \"yo\" || ok %v != true", v, ok)
		}
	})
}
