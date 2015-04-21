// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"sync"

	"github.com/syncthing/protocol"
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
	}
}

func (m *deviceActivity) leastBusy(availability map[protocol.DeviceID]uint32) (protocol.DeviceID, uint32) {
	m.mut.Lock()
	low := 2<<30 - 1
	var selected protocol.DeviceID
	var selectedFlags uint32 = 0
	for device, flags := range availability {
		if usage := m.act[device]; usage < low {
			low = usage
			selected = device
			selectedFlags = flags
		}
	}
	m.mut.Unlock()
	return selected, selectedFlags
}

func (m *deviceActivity) using(device protocol.DeviceID) {
	m.mut.Lock()
	m.act[device]++
	m.mut.Unlock()
}

func (m *deviceActivity) done(device protocol.DeviceID) {
	m.mut.Lock()
	m.act[device]--
	m.mut.Unlock()
}
