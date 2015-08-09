// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/protocol"
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

func (m *deviceActivity) leastBusy(availability []protocol.DeviceID) protocol.DeviceID {
	m.mut.Lock()
	low := 2<<30 - 1
	var selected protocol.DeviceID
	for _, device := range availability {
		if usage := m.act[device]; usage < low {
			low = usage
			selected = device
		}
	}
	m.mut.Unlock()
	return selected
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
