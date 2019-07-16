// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"sync"
	"time"
)

type DiscoverFunc func(renewal, timeout time.Duration) []Device

var providers []DiscoverFunc

func Register(provider DiscoverFunc) {
	providers = append(providers, provider)
}

func discoverAll(renewal, timeout time.Duration, stop chan struct{}) map[string]Device {
	wg := &sync.WaitGroup{}
	wg.Add(len(providers))

	c := make(chan Device)
	for _, discoverFunc := range providers {
		go func(f DiscoverFunc) {
			for _, dev := range f(renewal, timeout) {
				select {
				case c <- dev:
				case <-stop:
				}
			}
			wg.Done()
		}(discoverFunc)
	}

	nats := make(map[string]Device)

	for num := len(providers); len(nats) < num; {
		select {
		case dev := <-c:
			nats[dev.ID()] = dev
		case <-stop:
			return nil
		}
	}

	return nats
}
