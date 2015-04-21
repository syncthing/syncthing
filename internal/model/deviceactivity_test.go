// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"testing"

	"github.com/syncthing/protocol"
)

func TestDeviceActivity(t *testing.T) {
	n0 := protocol.DeviceID([32]byte{1, 2, 3, 4})
	n1 := protocol.DeviceID([32]byte{5, 6, 7, 8})
	n2 := protocol.DeviceID([32]byte{9, 10, 11, 12})
	devices := map[protocol.DeviceID]uint32{
		n0: 0,
		n1: 0,
		n2: 0,
	}
	na := newDeviceActivity()

	d1, _ := na.leastBusy(devices)
	na.using(d1)
	if lb, _ := na.leastBusy(devices); lb == d1 {
		t.Errorf("Least busy device should not be %v", d1)
	}

	d2, _ := na.leastBusy(devices)
	na.using(d2)
	if lb, _ := na.leastBusy(devices); lb == d1 || lb == d2 {
		t.Errorf("Least busy device should not be %v or %v", d1, d2)
	}

	na.done(d1)
	if lb, _ := na.leastBusy(devices); lb == d2 {
		t.Errorf("Least busy device should not be %v", d2)
	}
}
