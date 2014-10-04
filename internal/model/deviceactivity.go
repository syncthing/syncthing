// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
