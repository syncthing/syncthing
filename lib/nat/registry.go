// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"context"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/discover"
)

type DiscoverFunc func(ctx context.Context, renewal, timeout time.Duration, addrLister discover.AddressLister) []Device

var providers []DiscoverFunc

func Register(provider DiscoverFunc) {
	providers = append(providers, provider)
}

func discoverAll(ctx context.Context, renewal, timeout time.Duration, addrs discover.AddressLister) map[string]Device {
	wg := &sync.WaitGroup{}
	wg.Add(len(providers))

	c := make(chan Device)
	done := make(chan struct{})

	for _, discoverFunc := range providers {
		go func(f DiscoverFunc) {
			defer wg.Done()
			for _, dev := range f(ctx, renewal, timeout, addrs) {
				select {
				case c <- dev:
				case <-ctx.Done():
					return
				}
			}
		}(discoverFunc)
	}

	nats := make(map[string]Device)

	go func() {
		defer close(done)
		for {
			select {
			case dev, ok := <-c:
				if !ok {
					return
				}
				nats[dev.ID()] = dev
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	close(c)
	<-done

	return nats
}
