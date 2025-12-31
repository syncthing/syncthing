// Copyright (C) 2025 The Syncthing Authors & bxff
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/

package osutil

import (
	"path/filepath"
	"sync"

	"github.com/syncthing/syncthing/lib/fs"
)

// DirExistenceCache caches the list of file names in directories to avoid
// repeated Lstat syscalls when checking if files exist.
type DirExistenceCache struct {
	ffs   fs.Filesystem
	cache map[string]map[string]struct{} // dir -> set of names
	mu    sync.RWMutex
}

// NewDirExistenceCache creates a new cache for the given filesystem.
func NewDirExistenceCache(ffs fs.Filesystem) *DirExistenceCache {
	return &DirExistenceCache{
		ffs:   ffs,
		cache: make(map[string]map[string]struct{}),
	}
}

// FileExists checks if a file exists by looking up the directory contents.
// It caches DirNames results to reduce syscalls.
func (c *DirExistenceCache) FileExists(name string) (exists bool, err error) {
	dir := filepath.Dir(name)
	base := filepath.Base(name)

	// Fast path: check if we have a cached result
	c.mu.RLock()
	if names, ok := c.cache[dir]; ok {
		_, exists = names[base]
		c.mu.RUnlock()
		return exists, nil
	}
	c.mu.RUnlock()

	// Slow path: read directory contents and cache them
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if names, ok := c.cache[dir]; ok {
		_, exists = names[base]
		return exists, nil
	}

	// Read directory contents
	dirNames, err := c.ffs.DirNames(dir)
	if err != nil {
		if fs.IsNotExist(err) {
			// Directory doesn't exist, so file doesn't exist
			c.cache[dir] = make(map[string]struct{})
			return false, nil
		}
		return false, err
	}

	// Build the cache entry
	names := make(map[string]struct{}, len(dirNames))
	for _, n := range dirNames {
		names[n] = struct{}{}
	}
	c.cache[dir] = names

	_, exists = names[base]
	return exists, nil
}

// Invalidate clears the cache.
func (c *DirExistenceCache) Invalidate() {
	c.mu.Lock()
	c.cache = make(map[string]map[string]struct{})
	c.mu.Unlock()
}

// InvalidateDir removes a specific directory from the cache.
func (c *DirExistenceCache) InvalidateDir(dir string) {
	c.mu.Lock()
	delete(c.cache, dir)
	c.mu.Unlock()
}
