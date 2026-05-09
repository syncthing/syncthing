// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDeviceActivity(t *testing.T) {
	n0 := Availability{protocol.DeviceID([32]byte{1, 2, 3, 4}), false}
	n1 := Availability{protocol.DeviceID([32]byte{5, 6, 7, 8}), true}
	n2 := Availability{protocol.DeviceID([32]byte{9, 10, 11, 12}), false}
	devices := []Availability{n0, n1, n2}
	na := newDeviceActivity()

	if lb := na.leastBusy(devices); lb != 0 {
		t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
	}
	if lb := na.leastBusy(devices); lb != 1 {
		t.Errorf("Least busy device should rotate to n1 (%v) not %v", n1, lb)
	}
	if lb := na.leastBusy(devices); lb != 2 {
		t.Errorf("Least busy device should rotate to n2 (%v) not %v", n2, lb)
	}

	na.using(n0)
	if lb := na.leastBusy(devices); lb != 1 {
		t.Errorf("Least busy device should be n1 (%v) not %v", n1, lb)
	}

	na.using(n1)
	if lb := na.leastBusy(devices); lb != 2 {
		t.Errorf("Least busy device should be n2 (%v) not %v", n2, lb)
	}

	na.using(n2)
	if lb := na.leastBusy(devices); lb != 2 {
		t.Errorf("Least busy device should rotate to n2 (%v) not %v", n2, lb)
	}

	na.done(n1)
	if lb := na.leastBusy(devices); lb != 1 {
		t.Errorf("Least busy device should be n1 (%v) not %v", n1, lb)
	}

	na.done(n2)
	if lb := na.leastBusy(devices); lb != 1 {
		t.Errorf("Least busy device should still be n1 (%v) not %v", n1, lb)
	}

	na.done(n0)
	if lb := na.leastBusy(devices); lb != 2 {
		t.Errorf("Least busy device should rotate to n2 (%v) not %v", n2, lb)
	}
}

func TestDeviceActivityPrefersFasterTemporaryPeer(t *testing.T) {
	completeSlow := Availability{protocol.DeviceID([32]byte{1, 2, 3, 4}), false}
	partialFast := Availability{protocol.DeviceID([32]byte{5, 6, 7, 8}), true}
	devices := []Availability{completeSlow, partialFast}
	na := newDeviceActivity()

	na.success(completeSlow, 10_000_000, 1000)
	na.success(partialFast, 1_000_000, 1000)

	if lb := na.leastBusy(devices); lb != 1 {
		t.Errorf("Least busy device should be fast partial peer (%v) not %v", partialFast, lb)
	}
}
