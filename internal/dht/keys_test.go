// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"fmt"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDiscoveryKeyDerivation(t *testing.T) {
	id := protocol.NewDeviceID([]byte("test certificate"))
	date := time.Date(2026, time.November, 16, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		derive func(protocol.DeviceID, time.Time) [32]byte
		want   string
	}{
		{"record", deriveRecordKey, "e68aea52b9e561648533130b52900b3dfd82703bca433ff8501517cb310c257b"},
		{"encryption", deriveEncryptionKey, "ddc7588bf83d5f8fdf0311ba231a91b626eaea93a2d164765a4a472e546969c9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := tc.derive(id, date)
			if got := fmt.Sprintf("%x", key); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
			if tc.derive(id, date.Add(24*time.Hour-time.Nanosecond)) != key {
				t.Error("key changed within a UTC day")
			}
			if tc.derive(id, date.In(time.FixedZone("west", -7*60*60))) != key {
				t.Error("key depends on time zone")
			}
			if tc.derive(id, date.Add(24*time.Hour)) == key {
				t.Error("key did not change at UTC midnight")
			}
			otherID := id
			otherID[0] ^= 1
			if tc.derive(otherID, date) == key {
				t.Error("key did not change with device ID")
			}
		})
	}
	if deriveRecordKey(id, date) == deriveEncryptionKey(id, date) {
		t.Fatal("record and encryption keys must differ")
	}
}

func TestDiscoveryDates(t *testing.T) {
	for _, tc := range []struct{ hour, minute, count int }{
		{0, 0, 2}, {1, 59, 2}, {2, 0, 1}, {23, 59, 1},
	} {
		now := time.Date(2026, 11, 16, tc.hour, tc.minute, 0, 0, time.UTC)
		dates := discoveryDates(now.In(time.FixedZone("east", 9*3600)))
		if len(dates) != tc.count || !dates[0].Equal(now) {
			t.Fatalf("%v: %v", now, dates)
		}
		if len(dates) == 2 && dates[1].UTC().Day() != 15 {
			t.Fatal("wrong fallback day")
		}
	}
}

func TestDiscoveryKeyCache(t *testing.T) {
	var cache discoveryKeyCache
	id := protocol.NewDeviceID([]byte("cache test"))
	now := time.Now()
	first := cache.get(id, now)
	again := cache.get(id, now)
	if &first.signing[0] != &again.signing[0] {
		t.Fatal("keys were not cached")
	}
	if cache.get(id, now.Add(24*time.Hour)).record == first.record {
		t.Fatal("cached key did not rotate")
	}
}
