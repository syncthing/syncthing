// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type PullOrder int32

const (
	PullOrderRandom        PullOrder = 0
	PullOrderAlphabetic    PullOrder = 1
	PullOrderSmallestFirst PullOrder = 2
	PullOrderLargestFirst  PullOrder = 3
	PullOrderOldestFirst   PullOrder = 4
	PullOrderNewestFirst   PullOrder = 5
)

func (o PullOrder) String() string {
	switch o {
	case PullOrderRandom:
		return "random"
	case PullOrderAlphabetic:
		return "alphabetic"
	case PullOrderSmallestFirst:
		return "smallestFirst"
	case PullOrderLargestFirst:
		return "largestFirst"
	case PullOrderOldestFirst:
		return "oldestFirst"
	case PullOrderNewestFirst:
		return "newestFirst"
	default:
		return "unknown"
	}
}

func (o PullOrder) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

func (o *PullOrder) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "random":
		*o = PullOrderRandom
	case "alphabetic":
		*o = PullOrderAlphabetic
	case "smallestFirst":
		*o = PullOrderSmallestFirst
	case "largestFirst":
		*o = PullOrderLargestFirst
	case "oldestFirst":
		*o = PullOrderOldestFirst
	case "newestFirst":
		*o = PullOrderNewestFirst
	default:
		*o = PullOrderRandom
	}
	return nil
}
