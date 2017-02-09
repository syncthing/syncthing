// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type PullOrder int

const (
	OrderRandom PullOrder = iota // default is random
	OrderAlphabetic
	OrderSmallestFirst
	OrderLargestFirst
	OrderOldestFirst
	OrderNewestFirst
)

func (o PullOrder) String() string {
	switch o {
	case OrderRandom:
		return "random"
	case OrderAlphabetic:
		return "alphabetic"
	case OrderSmallestFirst:
		return "smallestFirst"
	case OrderLargestFirst:
		return "largestFirst"
	case OrderOldestFirst:
		return "oldestFirst"
	case OrderNewestFirst:
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
		*o = OrderRandom
	case "alphabetic":
		*o = OrderAlphabetic
	case "smallestFirst":
		*o = OrderSmallestFirst
	case "largestFirst":
		*o = OrderLargestFirst
	case "oldestFirst":
		*o = OrderOldestFirst
	case "newestFirst":
		*o = OrderNewestFirst
	default:
		*o = OrderRandom
	}
	return nil
}
