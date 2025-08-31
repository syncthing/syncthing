// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"testing"
	"time"
)

func TestMtimePairs(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal()
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t0 := time.Now().Truncate(time.Second)
	t1 := t0.Add(1234567890)

	// Set a pair
	if err := db.PutMtime("foo", "bar", t0, t1); err != nil {
		t.Fatal(err)
	}

	// Check it
	gt0, gt1 := db.GetMtime("foo", "bar")
	if !gt0.Equal(t0) || !gt1.Equal(t1) {
		t.Log(t0, gt0)
		t.Log(t1, gt1)
		t.Log("bad times")
	}

	// Delete it
	if err := db.DeleteMtime("foo", "bar"); err != nil {
		t.Fatal(err)
	}

	// Check it
	gt0, gt1 = db.GetMtime("foo", "bar")
	if !gt0.IsZero() || !gt1.IsZero() {
		t.Log(gt0, gt1)
		t.Log("bad times")
	}
}
