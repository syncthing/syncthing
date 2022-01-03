// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// deviceActivity tracks the number of outstanding requests per device and can
// answer which device is least busy. It is safe for use from multiple
// goroutines.
type deviceActivity struct {
	act map[protocol.DeviceID]int
	mut sync.Mutex
}

func newDeviceActivity() *deviceActivity {
	return &deviceActivity{
		act: make(map[protocol.DeviceID]int),
		mut: sync.NewMutex(),
	}
}

// Returns the index of the least busy device, or -1 if all are too busy.
func (m *deviceActivity) leastBusy(availability []Availability) int {
	m.mut.Lock()
	low := 2<<30 - 1
	best := -1
	for i := range availability {
		if usage := m.act[availability[i].ID]; usage < low {
			low = usage
			best = i
		}
	}
	m.mut.Unlock()
	return best
}

func (m *deviceActivity) using(availability Availability) {
	m.mut.Lock()
	m.act[availability.ID]++
	m.mut.Unlock()
}

func (m *deviceActivity) done(availability Availability) {
	m.mut.Lock()
	m.act[availability.ID]--
	m.mut.Unlock()
}
