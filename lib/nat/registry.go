// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/sentry"
)

type DiscoverFunc func(renewal, timeout time.Duration) []Device

var providers []DiscoverFunc

func Register(provider DiscoverFunc) {
	providers = append(providers, provider)
}

func discoverAll(renewal, timeout time.Duration) map[string]Device {
	wg := &sync.WaitGroup{}
	wg.Add(len(providers))

	c := make(chan Device)
	done := make(chan struct{})

	for _, discoverFunc := range providers {
		sentry.Go(func(f DiscoverFunc) func() {
			return func() {
				for _, dev := range f(renewal, timeout) {
					c <- dev
				}
				wg.Done()
			}
		}(discoverFunc))
	}

	nats := make(map[string]Device)

	sentry.Go(func() {
		for dev := range c {
			nats[dev.ID()] = dev
		}
		close(done)
	})

	wg.Wait()
	close(c)
	<-done

	return nats
}
