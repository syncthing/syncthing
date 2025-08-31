// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// The existence of this file means we get 0% test coverage rather than no
// test coverage at all. Remove when implementing an actual test.

package stats

import (
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/sqlite"
)

func TestDeviceStat(t *testing.T) {
	sdb, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sdb.Close()
	})

	sr := NewDeviceStatisticsReference(db.NewTyped(sdb, "devstatref"))
	if err := sr.WasSeen(); err != nil {
		t.Fatal(err)
	}
	if err := sr.LastConnectionDuration(42 * time.Second); err != nil {
		t.Fatal(err)
	}

	stat, err := sr.GetStatistics()
	if err != nil {
		t.Fatal(err)
	}

	if d := time.Since(stat.LastSeen); d > 5*time.Second {
		t.Error("Last seen far in the past:", d)
	}
	if d := stat.LastConnectionDurationS; d != 42 {
		t.Error("Bad last duration:", d)
	}
}
