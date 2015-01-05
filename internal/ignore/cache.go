// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
