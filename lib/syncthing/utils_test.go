// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"context"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/olddb"
	"github.com/syncthing/syncthing/internal/db/sqlite"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

func TestTryMigrateDatabaseMigratesDeviceStatistics(t *testing.T) {
	originalDataDir := locations.GetBaseDir(locations.DataBaseDir)
	if err := locations.SetBaseDir(locations.DataBaseDir, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := locations.SetBaseDir(locations.DataBaseDir, originalDataDir); err != nil {
			t.Error(err)
		}
	})

	legacyDB, err := leveldb.OpenFile(locations.Get(locations.LegacyDatabase), nil)
	if err != nil {
		t.Fatal(err)
	}
	device := protocol.NewDeviceID([]byte("test device"))
	want := time.Date(2025, 10, 17, 7, 50, 47, 0, time.UTC)
	value, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	key := append([]byte{olddb.KeyTypeDeviceStatistic}, device.String()...)
	key = append(key, "lastSeen"...)
	if err := legacyDB.Put(key, value, nil); err != nil {
		t.Fatal(err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := TryMigrateDatabase(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	sdb, err := sqlite.Open(locations.Get(locations.Database))
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	got, ok, err := db.NewTyped(sdb, "devicestats/"+device.String()).Time("lastSeen")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("last seen time was not migrated")
	}
	if !got.Equal(want) {
		t.Errorf("unexpected last seen time: got %v, want %v", got, want)
	}
}
