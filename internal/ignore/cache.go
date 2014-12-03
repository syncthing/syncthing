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

import (
	"sync"
	"time"
)

var (
	caches   = make(map[string]*cache)
	cacheMut sync.Mutex
)

func init() {
	// Periodically go through the cache and remove cache entries that have
	// not been touched in the last two hours.
	go cleanIgnoreCaches(2 * time.Hour)
}

type cache struct {
	patterns []Pattern
	entries  map[string]cacheEntry
	mut      sync.Mutex
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
	c.mut.Lock()
	for k, v := range c.entries {
		if time.Since(v.access) > d {
			delete(c.entries, k)
		}
	}
	c.mut.Unlock()
}

func (c *cache) get(key string) (result, ok bool) {
	c.mut.Lock()
	res, ok := c.entries[key]
	if ok {
		res.access = time.Now()
		c.entries[key] = res
	}
	c.mut.Unlock()
	return res.value, ok
}

func (c *cache) set(key string, val bool) {
	c.mut.Lock()
	c.entries[key] = cacheEntry{val, time.Now()}
	c.mut.Unlock()
}

func (c *cache) len() int {
	c.mut.Lock()
	l := len(c.entries)
	c.mut.Unlock()
	return l
}

func cleanIgnoreCaches(dur time.Duration) {
	for {
		time.Sleep(dur)
		cacheMut.Lock()
		for _, v := range caches {
			v.clean(dur)
		}
		cacheMut.Unlock()
	}
}
