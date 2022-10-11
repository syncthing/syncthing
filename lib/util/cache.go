// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package util

import "sync"

type Cache[K comparable, V any] struct {
	values map[K]V
	mutex  sync.RWMutex
}

func NewCache[K comparable, V any]() Cache[K, V] {
	return Cache[K, V]{
		values: make(map[K]V),
	}
}

func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	v, ok := c.values[k]
	return v, ok
}

func (c *Cache[K, V]) Set(k K, v V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.values[k] = v
}
