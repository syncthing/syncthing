// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"sync"

	"github.com/syncthing/syncthing/internal/protocol"
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

func (m deviceActivity) leastBusy(availability []protocol.DeviceID) protocol.DeviceID {
	m.mut.Lock()
	var low int = 2<<30 - 1
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

func (m deviceActivity) using(device protocol.DeviceID) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.act[device]++
}

func (m deviceActivity) done(device protocol.DeviceID) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.act[device]--
}
