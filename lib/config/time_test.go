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

type TestScheduleStruct struct {
	Schedule Schedule `default:"06:15-12:30 100 0"`
}

func TestScheduleDefaults(t *testing.T) {
	x := &TestScheduleStruct{}

	util.SetDefaults(x)

	if x.Schedule.Entries[0].StartHour != 6 {
		t.Error("not six")
	}
	if x.Schedule.Entries[0].StartMinute != 15 {
		t.Error("not fifteen")
	}
	if x.Schedule.Entries[0].EndHour != 12 {
		t.Error("not twelve")
	}
	if x.Schedule.Entries[0].EndMinute != 30 {
		t.Error("not thirty")
	}
	if x.Schedule.Entries[0].MaxSendKbps != 100 {
		t.Error("not hundred")
	}
	if x.Schedule.Entries[0].MaxRecvKbps != 0 {
		t.Error("not zero")
	}
}

func TestParseSchedule(t *testing.T) {
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
		_, err := ParseSchedule(tc.in)

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
