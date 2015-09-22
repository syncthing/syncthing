// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/thejerf/suture"
)

// The CachingMux aggregates results from multiple Finders. Each Finder has
// an associated cache time and negative cache time. The cache time sets how
// long we cache and return successfull lookup results, the negative cache
// time sets how long we refrain from asking about the same device ID after
// receiving a negative answer. The value of zero disables caching (positive
// or negative).
type CachingMux struct {
	*suture.Supervisor
	finders []cachedFinder
	caches  []*cache
	mut     sync.Mutex
}

// A cachedFinder is a Finder with associated cache timeouts.
type cachedFinder struct {
	Finder
	cacheTime    time.Duration
	negCacheTime time.Duration
}

func NewCachingMux() *CachingMux {
	return &CachingMux{
		Supervisor: suture.NewSimple("discover.cachingMux"),
		mut:        sync.NewMutex(),
	}
}

// Add registers a new Finder, with associated cache timeouts.
func (m *CachingMux) Add(finder Finder, cacheTime, negCacheTime time.Duration) {
	m.mut.Lock()
	m.finders = append(m.finders, cachedFinder{finder, cacheTime, negCacheTime})
	m.caches = append(m.caches, newCache())
	m.mut.Unlock()

	if svc, ok := finder.(suture.Service); ok {
		m.Supervisor.Add(svc)
	}
}

// Lookup attempts to resolve the device ID using any of the added Finders,
// while obeying the cache settings.
func (m *CachingMux) Lookup(deviceID protocol.DeviceID) (direct []string, relays []Relay, err error) {
	m.mut.Lock()
	for i, finder := range m.finders {
		if cacheEntry, ok := m.caches[i].Get(deviceID); ok {
			// We have a cache entry. Lets see what it says.

			if cacheEntry.found && time.Since(cacheEntry.when) < finder.cacheTime {
				// It's a positive, valid entry. Use it.
				if debug {
					l.Debugln("cached discovery entry for", deviceID, "at", finder.String())
					l.Debugln("   ", cacheEntry)
				}
				direct = append(direct, cacheEntry.Direct...)
				relays = append(relays, cacheEntry.Relays...)
				continue
			}

			if !cacheEntry.found && time.Since(cacheEntry.when) < finder.negCacheTime {
				// It's a negative, valid entry. We should not make another
				// attempt right now.
				if debug {
					l.Debugln("negative cache entry for", deviceID, "at", finder.String())
				}
				continue
			}

			// It's expired. Ignore and continue.
		}

		// Perform the actual lookup and cache the result.
		if td, tr, err := finder.Lookup(deviceID); err == nil {
			if debug {
				l.Debugln("lookup for", deviceID, "at", finder.String())
				l.Debugln("   ", td)
				l.Debugln("   ", tr)
			}
			direct = append(direct, td...)
			relays = append(relays, tr...)
			m.caches[i].Set(deviceID, CacheEntry{
				Direct: td,
				Relays: tr,
				when:   time.Now(),
				found:  len(td)+len(tr) > 0,
			})
		}
	}
	m.mut.Unlock()

	if debug {
		l.Debugln("lookup results for", deviceID)
		l.Debugln("   ", direct)
		l.Debugln("   ", relays)
	}

	return direct, relays, nil
}

func (m *CachingMux) String() string {
	return "discovery cache"
}

func (m *CachingMux) Error() error {
	return nil
}

func (m *CachingMux) ChildErrors() map[string]error {
	m.mut.Lock()
	children := make(map[string]error, len(m.finders))
	for _, f := range m.finders {
		children[f.String()] = f.Error()
	}
	m.mut.Unlock()
	return children
}

func (m *CachingMux) Cache() map[protocol.DeviceID]CacheEntry {
	// Res will be the "total" cache, i.e. the union of our cache and all our
	// children's caches.
	res := make(map[protocol.DeviceID]CacheEntry)

	m.mut.Lock()
	for i := range m.finders {
		// Each finder[i] has a corresponding cache at cache[i]. Go through it
		// and populate the total, if it's newer than what's already in there.
		// We skip any negative cache entries.
		for k, v := range m.caches[i].Cache() {
			if v.found && v.when.After(res[k].when) {
				res[k] = v
			}
		}

		// Then ask the finder itself for it's cache and do the same. If this
		// finder is a global discovery client, it will have no cache. If it's
		// a local discovery client, this will be it's current state.
		for k, v := range m.finders[i].Cache() {
			if v.found && v.when.After(res[k].when) {
				res[k] = v
			}
		}
	}
	m.mut.Unlock()

	return res
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
