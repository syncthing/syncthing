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

package stats

import (
	"time"

	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	deviceStatisticTypeLastSeen = iota
)

var deviceStatisticsTypes = []byte{
	deviceStatisticTypeLastSeen,
}

type DeviceStatistics struct {
	LastSeen time.Time
}

type DeviceStatisticsReference struct {
	db     *leveldb.DB
	device protocol.DeviceID
}

func NewDeviceStatisticsReference(db *leveldb.DB, device protocol.DeviceID) *DeviceStatisticsReference {
	return &DeviceStatisticsReference{
		db:     db,
		device: device,
	}
}

func (s *DeviceStatisticsReference) key(stat byte) []byte {
	k := make([]byte, 1+1+32)
	k[0] = keyTypeDeviceStatistic
	k[1] = stat
	copy(k[1+1:], s.device[:])
	return k
}

func (s *DeviceStatisticsReference) GetLastSeen() time.Time {
	value, err := s.db.Get(s.key(deviceStatisticTypeLastSeen), nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			l.Warnln("DeviceStatisticsReference: Failed loading last seen value for", s.device, ":", err)
		}
		return time.Unix(0, 0)
	}

	rtime := time.Time{}
	err = rtime.UnmarshalBinary(value)
	if err != nil {
		l.Warnln("DeviceStatisticsReference: Failed parsing last seen value for", s.device, ":", err)
		return time.Unix(0, 0)
	}
	if debug {
		l.Debugln("stats.DeviceStatisticsReference.GetLastSeen:", s.device, rtime)
	}
	return rtime
}

func (s *DeviceStatisticsReference) WasSeen() {
	if debug {
		l.Debugln("stats.DeviceStatisticsReference.WasSeen:", s.device)
	}
	value, err := time.Now().MarshalBinary()
	if err != nil {
		l.Warnln("DeviceStatisticsReference: Failed serializing last seen value for", s.device, ":", err)
		return
	}

	err = s.db.Put(s.key(deviceStatisticTypeLastSeen), value, nil)
	if err != nil {
		l.Warnln("Failed serializing last seen value for", s.device, ":", err)
	}
}

// Never called, maybe because it's worth while to keep the data
// or maybe because we have no easy way of knowing that a device has been removed.
func (s *DeviceStatisticsReference) Delete() error {
	for _, stype := range deviceStatisticsTypes {
		err := s.db.Delete(s.key(stype), nil)
		if debug && err == nil {
			l.Debugln("stats.DeviceStatisticsReference.Delete:", s.device, stype)
		}
		if err != nil && err != leveldb.ErrNotFound {
			return err
		}
	}
	return nil
}

func (s *DeviceStatisticsReference) GetStatistics() DeviceStatistics {
	return DeviceStatistics{
		LastSeen: s.GetLastSeen(),
	}
}
