// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"reflect"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDialQueueSort(t *testing.T) {
	t.Parallel()

	t.Run("ByLastSeen", func(t *testing.T) {
		t.Parallel()

		// Devices seen within the last week or so should be sorted stricly in order.
		now := time.Now()
		queue := dialQueue{
			{id: device1, lastSeen: now.Add(-5 * time.Hour)},  // 1
			{id: device2, lastSeen: now.Add(-50 * time.Hour)}, // 3
			{id: device3, lastSeen: now.Add(-25 * time.Hour)}, // 2
			{id: device4, lastSeen: now.Add(-2 * time.Hour)},  // 0
		}
		expected := []protocol.ShortID{device4.Short(), device1.Short(), device3.Short(), device2.Short()}

		queue.Sort()

		if !reflect.DeepEqual(shortDevices(queue), expected) {
			t.Error("expected different order")
		}
	})

	t.Run("OldConnections", func(t *testing.T) {
		t.Parallel()

		// Devices seen long ago should be randomized.
		now := time.Now()
		queue := dialQueue{
			{id: device1, lastSeen: now.Add(-5 * time.Hour)},       // 1
			{id: device2, lastSeen: now.Add(-50 * 24 * time.Hour)}, // 2, 3
			{id: device3, lastSeen: now.Add(-25 * 24 * time.Hour)}, // 2, 3
			{id: device4, lastSeen: now.Add(-2 * time.Hour)},       // 0
		}

		expected1 := []protocol.ShortID{device4.Short(), device1.Short(), device3.Short(), device2.Short()}
		expected2 := []protocol.ShortID{device4.Short(), device1.Short(), device2.Short(), device3.Short()}

		var seen1, seen2 int

		for i := 0; i < 100; i++ {
			queue.Sort()
			res := shortDevices(queue)
			if reflect.DeepEqual(res, expected1) {
				seen1++
				continue
			}
			if reflect.DeepEqual(res, expected2) {
				seen2++
				continue
			}
			t.Fatal("expected different order")
		}

		if seen1 < 10 || seen2 < 10 {
			t.Error("expected more even distribution", seen1, seen2)
		}
	})

	t.Run("ShortLivedConnections", func(t *testing.T) {
		t.Parallel()

		// Short lived connections should be sorted as if they were long ago
		now := time.Now()
		queue := dialQueue{
			{id: device1, lastSeen: now.Add(-5 * time.Hour)},                   // 1
			{id: device2, lastSeen: now.Add(-3 * time.Hour)},                   // 0
			{id: device3, lastSeen: now.Add(-25 * 24 * time.Hour)},             // 2, 3
			{id: device4, lastSeen: now.Add(-2 * time.Hour), shortLived: true}, // 2, 3
		}

		expected1 := []protocol.ShortID{device2.Short(), device1.Short(), device3.Short(), device4.Short()}
		expected2 := []protocol.ShortID{device2.Short(), device1.Short(), device4.Short(), device3.Short()}

		var seen1, seen2 int

		for i := 0; i < 100; i++ {
			queue.Sort()
			res := shortDevices(queue)
			if reflect.DeepEqual(res, expected1) {
				seen1++
				continue
			}
			if reflect.DeepEqual(res, expected2) {
				seen2++
				continue
			}
			t.Fatal("expected different order")
		}

		if seen1 < 10 || seen2 < 10 {
			t.Error("expected more even distribution", seen1, seen2)
		}
	})
}

func shortDevices(queue dialQueue) []protocol.ShortID {
	res := make([]protocol.ShortID, len(queue))
	for i, qe := range queue {
		res[i] = qe.id.Short()
	}
	return res
}
