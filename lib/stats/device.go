// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	lastSeenKey     = "lastSeen"
	connDurationKey = "lastConnDuration"
)

type DeviceStatistics struct {
	LastSeen                time.Time `json:"lastSeen"`
	LastConnectionDurationS float64   `json:"lastConnectionDurationS"`
}

type DeviceStatisticsReference struct {
	ns     *db.NamespacedKV
	device protocol.DeviceID
}

func NewDeviceStatisticsReference(dba backend.Backend, device protocol.DeviceID) *DeviceStatisticsReference {
	return &DeviceStatisticsReference{
		ns:     db.NewDeviceStatisticsNamespace(dba, device.String()),
		device: device,
	}
}

func (s *DeviceStatisticsReference) GetLastSeen() (time.Time, error) {
	t, ok, err := s.ns.Time(lastSeenKey)
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

func (s *DeviceStatisticsReference) GetLastConnectionDuration() (time.Duration, error) {
	d, ok, err := s.ns.Int64(connDurationKey)
	if err != nil {
		return 0, err
	} else if !ok {
		return 0, nil
	}
	l.Debugln("stats.DeviceStatisticsReference.GetLastConnectionDuration:", s.device, d)
	return time.Duration(d), nil
}

func (s *DeviceStatisticsReference) WasSeen() error {
	l.Debugln("stats.DeviceStatisticsReference.WasSeen:", s.device)
	return s.ns.PutTime(lastSeenKey, time.Now())
}

func (s *DeviceStatisticsReference) LastConnectionDuration(d time.Duration) error {
	l.Debugln("stats.DeviceStatisticsReference.LastConnectionDuration:", s.device, d)
	return s.ns.PutInt64(connDurationKey, d.Nanoseconds())
}

func (s *DeviceStatisticsReference) GetStatistics() (DeviceStatistics, error) {
	lastSeen, err := s.GetLastSeen()
	if err != nil {
		return DeviceStatistics{}, err
	}
	lastConnDuration, err := s.GetLastConnectionDuration()
	if err != nil {
		return DeviceStatistics{}, err
	}
	return DeviceStatistics{
		LastSeen:                lastSeen,
		LastConnectionDurationS: lastConnDuration.Seconds(),
	}, nil
}
