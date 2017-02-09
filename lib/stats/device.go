// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/lib/db"
)

type DeviceStatistics struct {
	LastSeen time.Time `json:"lastSeen"`
}

type DeviceStatisticsReference struct {
	ns     *db.NamespacedKV
	device string
}

func NewDeviceStatisticsReference(ldb *db.Instance, device string) *DeviceStatisticsReference {
	prefix := string(db.KeyTypeDeviceStatistic) + device
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
	l.Debugln("stats.DeviceStatisticsReference.GetLastSeen:", s.device, t)
	return t
}

func (s *DeviceStatisticsReference) WasSeen() {
	l.Debugln("stats.DeviceStatisticsReference.WasSeen:", s.device)
	s.ns.PutTime("lastSeen", time.Now())
}

func (s *DeviceStatisticsReference) GetStatistics() DeviceStatistics {
	return DeviceStatistics{
		LastSeen: s.GetLastSeen(),
	}
}
