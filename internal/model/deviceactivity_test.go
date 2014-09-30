// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/syncthing/syncthing/internal/protocol"
)

func TestDeviceActivity(t *testing.T) {
	n0 := protocol.DeviceID{1, 2, 3, 4}
	n1 := protocol.DeviceID{5, 6, 7, 8}
	n2 := protocol.DeviceID{9, 10, 11, 12}
	devices := []protocol.DeviceID{n0, n1, n2}
	na := newDeviceActivity()

	if lb := na.leastBusy(devices); lb != n0 {
		t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
	}
	if lb := na.leastBusy(devices); lb != n0 {
		t.Errorf("Least busy device should still be n0 (%v) not %v", n0, lb)
	}

	na.using(na.leastBusy(devices))
	if lb := na.leastBusy(devices); lb != n1 {
		t.Errorf("Least busy device should be n1 (%v) not %v", n1, lb)
	}

	na.using(na.leastBusy(devices))
	if lb := na.leastBusy(devices); lb != n2 {
		t.Errorf("Least busy device should be n2 (%v) not %v", n2, lb)
	}

	na.using(na.leastBusy(devices))
	if lb := na.leastBusy(devices); lb != n0 {
		t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
	}

	na.done(n1)
	if lb := na.leastBusy(devices); lb != n1 {
		t.Errorf("Least busy device should be n1 (%v) not %v", n1, lb)
	}

	na.done(n2)
	if lb := na.leastBusy(devices); lb != n1 {
		t.Errorf("Least busy device should still be n1 (%v) not %v", n1, lb)
	}

	na.done(n0)
	if lb := na.leastBusy(devices); lb != n0 {
		t.Errorf("Least busy device should be n0 (%v) not %v", n0, lb)
	}
}
