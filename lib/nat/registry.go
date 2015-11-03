// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package nat

import (
	"time"
)

type DiscoverFunc func(timeout time.Duration) []NATDevice

var providers []DiscoverFunc

func Register(provider DiscoverFunc) {
	providers = append(providers, provider)
}

func discoverAll(timeout time.Duration) map[string]NATDevice {
	natds := make(map[string]NATDevice)
	for _, discoverFunc := range providers {
		pantds := discoverFunc(timeout)
		for _, pnatd := range pantds {
			natds[pnatd.ID()] = pnatd
		}
	}
	return natds
}
