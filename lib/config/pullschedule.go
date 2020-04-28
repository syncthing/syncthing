// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type PullSchedule int

const (
	PullScheduleStandard PullSchedule = iota // default is standard
	PullScheduleRandom
	PullScheduleInOrder
)

func (o PullSchedule) String() string {
	switch o {
	case PullScheduleStandard:
		return "standard"
	case PullScheduleRandom:
		return "random"
	case PullScheduleInOrder:
		return "inOrder"
	default:
		return "unknown"
	}
}

func (o PullSchedule) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

func (o *PullSchedule) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "standard":
		*o = PullScheduleStandard
	case "random":
		*o = PullScheduleRandom
	case "inOrder":
		*o = PullScheduleInOrder
	default:
		*o = PullScheduleStandard
	}
	return nil
}
