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

func NewDeviceStatisticsReference(ldb *db.Lowlevel, device string) *DeviceStatisticsReference {
	return &DeviceStatisticsReference{
		ns:     db.NewDeviceStatisticsNamespace(ldb, device),
		device: device,
	}
}

func (s *DeviceStatisticsReference) GetLastSeen() (time.Time, error) {
	t, ok, err := s.ns.Time("lastSeen")
	if err != nil {
		return time.Time{}, err
	} else if !ok {
		// The default here is 1970-01-01 as opposed to the default
		// time.Time{} from s.ns
		return time.Unix(0, 0), nil
	}
	l.Debugln("stats.DeviceStatisticsReference.GetLastSeen:", s.device, t)
	return t, nil
}

func (s *DeviceStatisticsReference) WasSeen() error {
	l.Debugln("stats.DeviceStatisticsReference.WasSeen:", s.device)
	return s.ns.PutTime("lastSeen", time.Now())
}

func (s *DeviceStatisticsReference) GetStatistics() (DeviceStatistics, error) {
	lastSeen, err := s.GetLastSeen()
	if err != nil {
		return DeviceStatistics{}, err
	}
	return DeviceStatistics{
		LastSeen: lastSeen,
	}, nil
}
