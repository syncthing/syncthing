// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type Tuning int32

const (
	TuningAuto  Tuning = 0
	TuningSmall Tuning = 1
	TuningLarge Tuning = 2
)

func (t Tuning) String() string {
	switch t {
	case TuningAuto:
		return "auto"
	case TuningSmall:
		return "small"
	case TuningLarge:
		return "large"
	default:
		return "unknown"
	}
}

func (t Tuning) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *Tuning) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "auto":
		*t = TuningAuto
	case "small":
		*t = TuningSmall
	case "large":
		*t = TuningLarge
	default:
		*t = TuningAuto
	}
	return nil
}
