// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"sort"
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
	mut     sync.RWMutex
}

// A cachedFinder is a Finder with associated cache timeouts.
type cachedFinder struct {
	Finder
	cacheTime    time.Duration
	negCacheTime time.Duration
	priority     int
}

// A prioritizedAddress is what we use to sort addresses returned from
// different sources with different priorities.
type prioritizedAddress struct {
	priority int
	addr     string
}

// An error may implement cachedError, in which case it will be interrogated
// to see how long we should cache the error. This overrides the default
// negative cache time.
type cachedError interface {
	CacheFor() time.Duration
}

func NewCachingMux() *CachingMux {
	return &CachingMux{
		Supervisor: suture.NewSimple("discover.cachingMux"),
		mut:        sync.NewRWMutex(),
	}
}

// Add registers a new Finder, with associated cache timeouts.
func (m *CachingMux) Add(finder Finder, cacheTime, negCacheTime time.Duration, priority int) {
	m.mut.Lock()
	m.finders = append(m.finders, cachedFinder{finder, cacheTime, negCacheTime, priority})
	m.caches = append(m.caches, newCache())
	m.mut.Unlock()

	if service, ok := finder.(suture.Service); ok {
		m.Supervisor.Add(service)
	}
}

// Lookup attempts to resolve the device ID using any of the added Finders,
// while obeying the cache settings.
func (m *CachingMux) Lookup(deviceID protocol.DeviceID) (direct []string, relays []Relay, err error) {
	var pdirect []prioritizedAddress

	m.mut.RLock()
	for i, finder := range m.finders {
		if cacheEntry, ok := m.caches[i].Get(deviceID); ok {
			// We have a cache entry. Lets see what it says.

			if cacheEntry.found && time.Since(cacheEntry.when) < finder.cacheTime {
				// It's a positive, valid entry. Use it.
				l.Debugln("cached discovery entry for", deviceID, "at", finder)
				l.Debugln("  cache:", cacheEntry)
				for _, addr := range cacheEntry.Direct {
					pdirect = append(pdirect, prioritizedAddress{finder.priority, addr})
				}
				relays = append(relays, cacheEntry.Relays...)
				continue
			}

			valid := time.Now().Before(cacheEntry.validUntil) || time.Since(cacheEntry.when) < finder.negCacheTime
			if !cacheEntry.found && valid {
				// It's a negative, valid entry. We should not make another
				// attempt right now.
				l.Debugln("negative cache entry for", deviceID, "at", finder, "valid until", cacheEntry.when.Add(finder.negCacheTime), "or", cacheEntry.validUntil)
				continue
			}

			// It's expired. Ignore and continue.
		}

		// Perform the actual lookup and cache the result.
		if td, tr, err := finder.Lookup(deviceID); err == nil {
			l.Debugln("lookup for", deviceID, "at", finder)
			l.Debugln("  direct:", td)
			l.Debugln("  relays:", tr)
			for _, addr := range td {
				pdirect = append(pdirect, prioritizedAddress{finder.priority, addr})
			}
			relays = append(relays, tr...)
			m.caches[i].Set(deviceID, CacheEntry{
				Direct: td,
				Relays: tr,
				when:   time.Now(),
				found:  len(td)+len(tr) > 0,
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
			m.caches[i].Set(deviceID, entry)
		}
	}
	m.mut.RUnlock()

	direct = uniqueSortedAddrs(pdirect)
	relays = uniqueSortedRelays(relays)
	l.Debugln("lookup results for", deviceID)
	l.Debugln("  direct: ", direct)
	l.Debugln("  relays: ", relays)

	return direct, relays, nil
}

func (m *CachingMux) String() string {
	return "discovery cache"
}

func (m *CachingMux) Error() error {
	return nil
}

func (m *CachingMux) ChildErrors() map[string]error {
	children := make(map[string]error, len(m.finders))
	m.mut.RLock()
	for _, f := range m.finders {
		children[f.String()] = f.Error()
	}
	m.mut.RUnlock()
	return children
}

func (m *CachingMux) Cache() map[protocol.DeviceID]CacheEntry {
	// Res will be the "total" cache, i.e. the union of our cache and all our
	// children's caches.
	res := make(map[protocol.DeviceID]CacheEntry)

	m.mut.RLock()
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
	m.mut.RUnlock()

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

func uniqueSortedAddrs(ss []prioritizedAddress) []string {
	// We sort the addresses by priority, then filter them based on seen
	// (first time seen is the on kept, so we retain priority).
	sort.Sort(prioritizedAddressList(ss))
	filtered := make([]string, 0, len(ss))
	seen := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		if _, ok := seen[s.addr]; !ok {
			filtered = append(filtered, s.addr)
			seen[s.addr] = struct{}{}
		}
	}
	return filtered
}

func uniqueSortedRelays(rs []Relay) []Relay {
	m := make(map[string]Relay, len(rs))
	for _, r := range rs {
		m[r.URL] = r
	}

	var ur = make([]Relay, 0, len(m))
	for _, r := range m {
		ur = append(ur, r)
	}

	sort.Sort(relayList(ur))

	return ur
}

type relayList []Relay

func (l relayList) Len() int {
	return len(l)
}

func (l relayList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l relayList) Less(a, b int) bool {
	return l[a].URL < l[b].URL
}

type prioritizedAddressList []prioritizedAddress

func (l prioritizedAddressList) Len() int {
	return len(l)
}

func (l prioritizedAddressList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l prioritizedAddressList) Less(a, b int) bool {
	if l[a].priority != l[b].priority {
		return l[a].priority < l[b].priority
	}
	return l[a].addr < l[b].addr
}
