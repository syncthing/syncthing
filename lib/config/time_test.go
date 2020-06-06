// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"testing"

	"github.com/syncthing/syncthing/lib/util"
)

type TestSchedulesStruct struct {
	Schedules Schedules `default:"06:15-12:30 100 0"`
}

func TestSchedulesDefaults(t *testing.T) {
	x := &TestSchedulesStruct{}

	util.SetDefaults(x)

	if x.Schedules.RatesSchedule.Time.StartHour != 6 {
		t.Error("not six")
	}
	if x.Schedules.RatesSchedule.Time.StartMinute != 15 {
		t.Error("not fifteen")
	}
	if x.Schedules.RatesSchedule.Time.EndHour != 12 {
		t.Error("not twelve")
	}
	if x.Schedules.RatesSchedule.Time.EndMinute != 30 {
		t.Error("not thirty")
	}
	if x.Schedules.RatesSchedule.MaxSendKbps != 100 {
		t.Error("not hundred")
	}
	if x.Schedules.RatesSchedule.MaxRecvKbps != 0 {
		t.Error("not zero")
	}
}

func TestParseSchedules(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"06:15-12:30 100", false},
		{"06:15-12:30", false},
		{"06:15-12 100 0", false},
		{"06:15 100 0", false},
		{"06:15 12:30 100 0", false},
		{"06:05-12:30 100 0", true},
		{"6:5-12:30 100 0", true},
		{"6:05-12:30 100 0", true},
		{"06:5-12:30 100 0", true},
	}

	for _, tc := range cases {
		_, err := ParseSchedules(tc.in)

		if !tc.ok {
			if err == nil {
				t.Errorf("Unexpected nil error in UnmarshalText(%q)", tc.in)
			}
			continue
		}

		if err != nil {
			t.Errorf("Unexpected error in UnmarshalText(%q): %v", tc.in, err)
			continue
		}
	}
}
