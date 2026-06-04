// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestIndexIDs(t *testing.T) {
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

	t.Run("LocalDeviceID", func(t *testing.T) {
		t.Parallel()

		localID, err := db.GetIndexID("foo", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if localID == 0 {
			t.Fatal("should have been generated")
		}

		again, err := db.GetIndexID("foo", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if again != localID {
			t.Fatal("should get same again")
		}

		other, err := db.GetIndexID("bar", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if other == localID {
			t.Fatal("should not get same for other folder")
		}
	})

	t.Run("OtherDeviceID", func(t *testing.T) {
		t.Parallel()

		localID, err := db.GetIndexID("foo", protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if localID != 0 {
			t.Fatal("should have been zero")
		}

		newID := protocol.NewIndexID()
		if err := db.SetIndexID("foo", protocol.DeviceID{42}, newID); err != nil {
			t.Fatal(err)
		}

		again, err := db.GetIndexID("foo", protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if again != newID {
			t.Log(again, newID)
			t.Fatal("should get the ID we set")
		}
	})

	t.Run("DropAllFilesClearsOtherDeviceID", func(t *testing.T) {
		t.Parallel()

		device := protocol.DeviceID{99}
		newID := protocol.NewIndexID()

		if err := db.Update("drop-index-id", device, []protocol.FileInfo{
			genFile("test1", 1, 1),
		}); err != nil {
			t.Fatal(err)
		}

		if err := db.SetIndexID("drop-index-id", device, newID); err != nil {
			t.Fatal(err)
		}

		again, err := db.GetIndexID("drop-index-id", device)
		if err != nil {
			t.Fatal(err)
		}
		if again != newID {
			t.Log(again, newID)
			t.Fatal("should get the ID we set")
		}

		if err := db.DropAllFiles("drop-index-id", device); err != nil {
			t.Fatal(err)
		}

		dropped, err := db.GetIndexID("drop-index-id", device)
		if err != nil {
			t.Fatal(err)
		}
		if dropped != 0 {
			t.Log(dropped)
			t.Fatal("should clear the ID when dropping all files")
		}
	})
}
