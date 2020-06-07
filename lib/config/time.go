// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

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

func (s Schedules) IsRatesScheduleEnabled() bool {
	t := s.RatesSchedule.Time
	return t.StartHour != t.EndHour || t.StartMinute != t.EndMinute
}
