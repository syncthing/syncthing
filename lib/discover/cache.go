// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"context"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// A cachedFinder is a Finder with associated cache timeouts.
type cachedFinder struct {
	Finder

	cacheTime    time.Duration
	negCacheTime time.Duration
	cache        *cache
}

// An error may implement cachedError, in which case it will be interrogated
// to see how long we should cache the error. This overrides the default
// negative cache time.
type cachedError interface {
	CacheFor() time.Duration
}

func (f *cachedFinder) Lookup(ctx context.Context, deviceID protocol.DeviceID) ([]string, error) {
	if cacheEntry, ok := f.cache.Get(deviceID); ok {
		// We have a cache entry. Lets see what it says.

		if cacheEntry.found && time.Since(cacheEntry.when) < f.cacheTime {
			// It's a positive, valid entry. Use it.
			slog.DebugContext(ctx, "Found cached discovery entry", "device", deviceID, "finder", f, "entry", cacheEntry)
			return cacheEntry.Addresses, nil
		}

		valid := time.Now().Before(cacheEntry.validUntil) || time.Since(cacheEntry.when) < f.negCacheTime
		if !cacheEntry.found && valid {
			// It's a negative, valid entry. We should not make another
			// attempt right now.
			slog.DebugContext(ctx, "Negative cache entry", "device", deviceID, "finder", f, "until1", cacheEntry.when.Add(f.negCacheTime), "until2", cacheEntry.validUntil)
			return nil, nil
		}
	}

	// Perform the actual lookup and cache the result.
	addrs, err := f.Finder.Lookup(ctx, deviceID)
	if err == nil {
		slog.DebugContext(ctx, "Got finder result", "device", deviceID, "finder", f, "address", addrs)
		f.cache.Set(deviceID, CacheEntry{
			Addresses: addrs,
			when:      time.Now(),
			found:     len(addrs) > 0,
		})
	} else {
		// Lookup returned error, add a negative cache entry.
		entry := CacheEntry{
			when:  time.Now(),
			found: false,
		}
		if err, ok := err.(cachedError); ok {
			entry.validUntil = time.Now().Add(err.CacheFor())
		}
		f.cache.Set(deviceID, entry)
	}

	return addrs, err
}

// A cache can be embedded wherever useful

type cache struct {
	entries map[protocol.DeviceID]CacheEntry
	mut     sync.Mutex
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
	maps.Copy(m, c.entries)
	c.mut.Unlock()
	return m
}
