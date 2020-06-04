// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Schedule struct {
	Entries []ScheduleEntry `json:"entry" xml:"entry"`
}

type ScheduleEntry struct {
	StartHour   int `json:"startHour" xml:"startHour"`
	StartMinute int `json:"startMinute" xml:"startMinute"`
	StartDay    int `json:"startDay" xml:"startDay"`
	EndHour     int `json:"endHour" xml:"endHour"`
	EndMinute   int `json:"endMinute" xml:"endMinute"`
	EndDay      int `json:"endDay" xml:"endDay"`
	MaxSendKbps int `json:"maxSendKbps" xml:"maxSendKbps"`
	MaxRecvKbps int `json:"maxRecvKbps" xml:"maxRecvKbps"`
}

func ParseSchedule(s string) (Schedule, error) {
	fields := strings.Split(s, " ")
	if len(fields) != 3 {
		return Schedule{}, fmt.Errorf("Wrong Schedule format")
	}

	times := strings.Split(fields[0], "-")
	if len(times) != 2 {
		return Schedule{}, fmt.Errorf("Wrong Schedule format")
	}

	begin := strings.Split(times[0], ":")
	if len(begin) != 2 {
		return Schedule{}, fmt.Errorf("Wrong Schedule format")
	}

	end := strings.Split(times[1], ":")
	if len(end) != 2 {
		return Schedule{}, fmt.Errorf("Wrong Schedule format")
	}

	sch := ScheduleEntry{}
	var err error

	if sch.StartHour, err = strconv.Atoi(begin[0]); err != nil {
		return Schedule{}, err
	}
	if sch.StartMinute, err = strconv.Atoi(begin[1]); err != nil {
		return Schedule{}, err
	}
	if sch.EndHour, err = strconv.Atoi(end[0]); err != nil {
		return Schedule{}, err
	}
	if sch.EndMinute, err = strconv.Atoi(end[1]); err != nil {
		return Schedule{}, err
	}
	if sch.MaxSendKbps, err = strconv.Atoi(fields[1]); err != nil {
		return Schedule{}, err
	}
	if sch.MaxRecvKbps, err = strconv.Atoi(fields[2]); err != nil {
		return Schedule{}, err
	}

	return Schedule{[]ScheduleEntry{sch}}, nil
}

func (s Schedule) String() string {
	return fmt.Sprintf("%d:%d-%d:%d %d %d", s.Entries[0].StartHour, s.Entries[0].StartMinute,
		s.Entries[0].EndHour, s.Entries[0].EndMinute, s.Entries[0].MaxSendKbps, s.Entries[0].MaxRecvKbps)
}

func (s *Schedule) ParseDefault(str string) error {
	sz, err := ParseSchedule(str)
	*s = sz
	return err
}
