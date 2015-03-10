// Copyright (C) 2014 The Syncthing Authors.
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

package stats

import (
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syndtr/goleveldb/leveldb"
)

type DeviceStatistics struct {
	LastSeen time.Time `json:"lastSeen"`
}

type DeviceStatisticsReference struct {
	ns     *db.NamespacedKV
	device protocol.DeviceID
}

func NewDeviceStatisticsReference(ldb *leveldb.DB, device protocol.DeviceID) *DeviceStatisticsReference {
	prefix := string(db.KeyTypeDeviceStatistic) + device.String()
	return &DeviceStatisticsReference{
		ns:     db.NewNamespacedKV(ldb, prefix),
		device: device,
	}
}

func (s *DeviceStatisticsReference) GetLastSeen() time.Time {
	t, ok := s.ns.Time("lastSeen")
	if !ok {
		// The default here is 1970-01-01 as opposed to the default
		// time.Time{} from s.ns
		return time.Unix(0, 0)
	}
	if debug {
		l.Debugln("stats.DeviceStatisticsReference.GetLastSeen:", s.device, t)
	}
	return t
}

func (s *DeviceStatisticsReference) WasSeen() {
	if debug {
		l.Debugln("stats.DeviceStatisticsReference.WasSeen:", s.device)
	}
	s.ns.PutTime("lastSeen", time.Now())
}

func (s *DeviceStatisticsReference) GetStatistics() DeviceStatistics {
	return DeviceStatistics{
		LastSeen: s.GetLastSeen(),
	}
}
