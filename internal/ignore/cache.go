// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package ignore

import "time"

type cache struct {
	patterns []Pattern
	entries  map[string]cacheEntry
}

type cacheEntry struct {
	value  bool
	access time.Time
}

func newCache(patterns []Pattern) *cache {
	return &cache{
		patterns: patterns,
		entries:  make(map[string]cacheEntry),
	}
}

func (c *cache) clean(d time.Duration) {
	for k, v := range c.entries {
		if time.Since(v.access) > d {
			delete(c.entries, k)
		}
	}
}

func (c *cache) get(key string) (result, ok bool) {
	res, ok := c.entries[key]
	if ok {
		res.access = time.Now()
		c.entries[key] = res
	}
	return res.value, ok
}

func (c *cache) set(key string, val bool) {
	c.entries[key] = cacheEntry{val, time.Now()}
}

func (c *cache) len() int {
	l := len(c.entries)
	return l
}
