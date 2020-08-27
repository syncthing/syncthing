// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	stdsync "sync"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/protocol"
)

// A cachedFinder is a Finder with associated cache timeouts.
type cachedFinder struct {
	Finder
	cacheTime    time.Duration
	negCacheTime time.Duration
	cache        *cache
	token        *suture.ServiceToken
}

// An error may implement cachedError, in which case it will be interrogated
// to see how long we should cache the error. This overrides the default
// negative cache time.
type cachedError interface {
	CacheFor() time.Duration
}

// A cache can be embedded wherever useful

type cache struct {
	entries map[protocol.DeviceID]CacheEntry
	mut     stdsync.Mutex
}

func newCache() *cache {
	return &cache{
		entries: make(map[protocol.DeviceID]CacheEntry),
	}
}

func (c *cache) Set(id protocol.DeviceID, ce CacheEntry) {
	c.mut.Lock()
	c.entries[id] = ce
	c.mut.Unlock()
}

func (c *cache) Get(id protocol.DeviceID) (CacheEntry, bool) {
	c.mut.Lock()
	ce, ok := c.entries[id]
	c.mut.Unlock()
	return ce, ok
}

func (c *cache) Cache() map[protocol.DeviceID]CacheEntry {
	c.mut.Lock()
	m := make(map[protocol.DeviceID]CacheEntry, len(c.entries))
	for k, v := range c.entries {
		m[k] = v
	}
	c.mut.Unlock()
	return m
}
