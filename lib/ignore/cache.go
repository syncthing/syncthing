// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import "time"

type nower interface {
	Now() time.Time
}

var clock = nower(defaultClock{})

type cache struct {
	patterns []Pattern
	entries  map[string]cacheEntry
}

type cacheEntry struct {
	result Result
	access int64 // Unix nanosecond count. Sufficient until the year 2262.
}

func newCache(patterns []Pattern) *cache {
	return &cache{
		patterns: patterns,
		entries:  make(map[string]cacheEntry),
	}
}

func (c *cache) clean(d time.Duration) {
	for k, v := range c.entries {
		if clock.Now().Sub(time.Unix(0, v.access)) > d {
			delete(c.entries, k)
		}
	}
}

func (c *cache) get(key string) (Result, bool) {
	entry, ok := c.entries[key]
	if ok {
		entry.access = clock.Now().UnixNano()
		c.entries[key] = entry
	}
	return entry.result, ok
}

func (c *cache) set(key string, result Result) {
	c.entries[key] = cacheEntry{result, time.Now().UnixNano()}
}

func (c *cache) len() int {
	l := len(c.entries)
	return l
}

type defaultClock struct{}

func (defaultClock) Now() time.Time {
	return time.Now()
}
