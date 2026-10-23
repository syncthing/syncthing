// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"slices"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDeviceActivity(t *testing.T) {
	n0 := Availability{protocol.DeviceID([32]byte{1, 2, 3, 4}), false}
	n1 := Availability{protocol.DeviceID([32]byte{5, 6, 7, 8}), true}
	n2 := Availability{protocol.DeviceID([32]byte{9, 10, 11, 12}), false}
	devices := []Availability{n0, n1, n2}

	t.Run("basic", func(t *testing.T) {
		na := newDeviceActivity()

		// making blocks take the assumed rate means we take device rate out
		// of the equation for this test, basing it only on the number of
		// outstanding blocks
		blockDur := time.Second * protocol.MinBlockSize / assumedRate
		start := time.Now()

		if lb := na.leastBusy(devices); lb != 0 {
			t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
		}
		if lb := na.leastBusy(devices); lb != 0 {
			t.Errorf("Least busy device should still be n0 (%v) not %v", n0, lb)
		}

		lb := na.leastBusy(devices)
		t0 := na.using(devices[lb].ID, protocol.MinBlockSize, start)
		if lb := na.leastBusy(devices); lb != 1 {
			t.Errorf("Least busy device should be n1 (%v) not %v", n1, lb)
		}

		lb = na.leastBusy(devices)
		t1 := na.using(devices[lb].ID, protocol.MinBlockSize, start)
		if lb := na.leastBusy(devices); lb != 2 {
			t.Errorf("Least busy device should be n2 (%v) not %v", n2, lb)
		}

		lb = na.leastBusy(devices)
		t2 := na.using(devices[lb].ID, protocol.MinBlockSize, start)
		if lb := na.leastBusy(devices); lb != 0 {
			t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
		}

		na.done(t1, start.Add(blockDur))
		if lb := na.leastBusy(devices); lb != 1 {
			t.Errorf("Least busy device should be n1 (%v) not %v", n1, lb)
		}

		na.done(t2, start.Add(blockDur))
		if lb := na.leastBusy(devices); lb != 1 {
			t.Errorf("Least busy device should still be n1 (%v) not %v", n1, lb)
		}

		na.done(t0, start.Add(blockDur))
		if lb := na.leastBusy(devices); lb != 0 {
			t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
		}
	})

	t.Run("rateBased", func(t *testing.T) {
		na := newDeviceActivity()
		start := time.Now()

		// n0 has proven to be quick, averaging ten blocks per second
		t0 := na.using(n0.ID, protocol.MinBlockSize, start)
		na.done(t0, start.Add(time.Second/10))

		// n1 is a bit slower, averaging two blocks per second
		t1 := na.using(n1.ID, protocol.MinBlockSize, start)
		na.done(t1, start.Add(time.Second/2))

		// n2 is a yet slower, averaging one block per second
		t3 := na.using(n2.ID, protocol.MinBlockSize, start)
		na.done(t3, start.Add(time.Second/1))

		// Request one hundred blocks, and observe the distribution
		count := make([]int, 3)
		for range 100 {
			idx := na.leastBusy(devices)
			count[idx]++
			na.using(devices[idx].ID, protocol.MinBlockSize, start)
		}

		// n0 should have been assigned 10/13 ~= 78% of the blocks
		// n1 should have been assigned 2/13 ~= 15% of the blocks
		// n2 should have been assigned 1/13 ~= 7% of the blocks
		exp := []int{78, 15, 7}

		if !slices.Equal(count, exp) {
			t.Error("Unexpected results", count)
		}
	})
}

func TestRateSlicing(t *testing.T) {
	s := []usageInterval{
		{
			start: time.Unix(100000000, 0),
			end:   time.Unix(100001000, 0),
			rate:  100,
		},
		{
			start: time.Unix(100000500, 0),
			end:   time.Unix(100002500, 0),
			rate:  125,
		},
		{
			start: time.Unix(100002000, 0),
			end:   time.Unix(100003000, 0),
			rate:  150,
		},
	}

	// t.Log(s)
	res := sliceRates(s)
	_ = res
	// t.Log(res)
}
