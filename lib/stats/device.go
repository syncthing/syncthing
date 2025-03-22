// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/internal/db"
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
	kv *db.Typed
}

func NewDeviceStatisticsReference(kv *db.Typed) *DeviceStatisticsReference {
	return &DeviceStatisticsReference{
		kv: kv,
	}
}

func (s *DeviceStatisticsReference) GetLastSeen() (time.Time, error) {
	t, ok, err := s.kv.Time(lastSeenKey)
	if err != nil {
		return time.Time{}, err
	} else if !ok {
		// The default here is 1970-01-01 as opposed to the default
		// time.Time{} from s.ns
		return time.Unix(0, 0), nil
	}
	return t, nil
}

func (s *DeviceStatisticsReference) GetLastConnectionDuration() (time.Duration, error) {
	d, ok, err := s.kv.Int64(connDurationKey)
	if err != nil {
		return 0, err
	} else if !ok {
		return 0, nil
	}
	return time.Duration(d), nil
}

func (s *DeviceStatisticsReference) WasSeen() error {
	return s.kv.PutTime(lastSeenKey, time.Now().Truncate(time.Second))
}

func (s *DeviceStatisticsReference) LastConnectionDuration(d time.Duration) error {
	return s.kv.PutInt64(connDurationKey, d.Nanoseconds())
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
