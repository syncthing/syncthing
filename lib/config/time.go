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

type Schedules struct {
	RatesSchedule RatesSchedule `json:"ratesSchedule" xml:"ratesSchedule"`
}

type RatesSchedule struct {
	Time        TimeFrame `json:"time" xml:"time"`
	MaxSendKbps int       `json:"maxSendKbps" xml:"maxSendKbps"`
	MaxRecvKbps int       `json:"maxRecvKbps" xml:"maxRecvKbps"`
}

type TimeFrame struct {
	StartHour   int `json:"startHour" xml:"startHour"`
	StartMinute int `json:"startMinute" xml:"startMinute"`
	EndHour     int `json:"endHour" xml:"endHour"`
	EndMinute   int `json:"endMinute" xml:"endMinute"`
}

func ParseSchedules(s string) (Schedules, error) {
	fields := strings.Split(s, " ")
	if len(fields) != 3 {
		return Schedules{}, fmt.Errorf("Wrong Schedules format")
	}

	times := strings.Split(fields[0], "-")
	if len(times) != 2 {
		return Schedules{}, fmt.Errorf("Wrong Schedules format")
	}

	begin := strings.Split(times[0], ":")
	if len(begin) != 2 {
		return Schedules{}, fmt.Errorf("Wrong Schedules format")
	}

	end := strings.Split(times[1], ":")
	if len(end) != 2 {
		return Schedules{}, fmt.Errorf("Wrong Schedules format")
	}

	tf := TimeFrame{}
	var err error

	if tf.StartHour, err = strconv.Atoi(begin[0]); err != nil {
		return Schedules{}, err
	}
	if tf.StartMinute, err = strconv.Atoi(begin[1]); err != nil {
		return Schedules{}, err
	}
	if tf.EndHour, err = strconv.Atoi(end[0]); err != nil {
		return Schedules{}, err
	}
	if tf.EndMinute, err = strconv.Atoi(end[1]); err != nil {
		return Schedules{}, err
	}

	rs := RatesSchedule{
		Time: tf,
	}

	if rs.MaxSendKbps, err = strconv.Atoi(fields[1]); err != nil {
		return Schedules{}, err
	}
	if rs.MaxRecvKbps, err = strconv.Atoi(fields[2]); err != nil {
		return Schedules{}, err
	}

	return Schedules{rs}, nil
}

func (s Schedules) String() string {
	return fmt.Sprintf("%d:%d-%d:%d %d %d", s.RatesSchedule.Time.StartHour, s.RatesSchedule.Time.StartMinute,
		s.RatesSchedule.Time.EndHour, s.RatesSchedule.Time.EndMinute, s.RatesSchedule.MaxSendKbps, s.RatesSchedule.MaxRecvKbps)
}

func (s Schedules) IsEnabled() bool {
	t := s.RatesSchedule.Time
	return t.StartHour != t.EndHour || t.StartMinute != t.EndMinute
}

func (s *Schedules) ParseDefault(str string) error {
	sz, err := ParseSchedules(str)
	*s = sz
	return err
}
