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

// SymlinkCache caches the results of TraversesSymlink checks for path components
// to avoid redundant Lstat syscalls when checking multiple files in the same
// directory hierarchy.
type SymlinkCache struct {
	ffs   fs.Filesystem
	cache map[string]symlinkCacheEntry
	mu    sync.RWMutex
}

type symlinkCacheEntry struct {
	isSymlink bool
	isNotDir  bool
	err       error
}

// NewSymlinkCache creates a new cache for the given filesystem.
func NewSymlinkCache(ffs fs.Filesystem) *SymlinkCache {
	return &SymlinkCache{
		ffs:   ffs,
		cache: make(map[string]symlinkCacheEntry),
	}
}

// TraversesSymlinkCached checks if any path component is a symlink, using cached results.
func (c *SymlinkCache) TraversesSymlinkCached(name string) error {
	if name == "" || name == "." {
		return nil
	}

	// Check each path component
	components := fs.PathComponents(name)
	path := ""

	for _, comp := range components {
		if path == "" {
			path = comp
		} else {
			path = filepath.Join(path, comp)
		}

		// Fast path: check cache
		c.mu.RLock()
		entry, cached := c.cache[path]
		c.mu.RUnlock()

		if cached {
			if entry.isSymlink {
				return &TraversesSymlinkError{path: path}
			}
			if entry.isNotDir {
				return &NotADirectoryError{path: path}
			}
			if entry.err != nil {
				return entry.err
			}
			continue
		}

		// Slow path: Lstat and cache
		info, err := c.ffs.Lstat(path)

		c.mu.Lock()
		if err != nil {
			c.cache[path] = symlinkCacheEntry{err: err}
			c.mu.Unlock()
			if fs.IsNotExist(err) {
				return &NotADirectoryError{path: path}
			}
			return err
		}

		isSymlink := info.IsSymlink()
		isDir := info.IsDir()

		c.cache[path] = symlinkCacheEntry{
			isSymlink: isSymlink,
			isNotDir:  !isDir && !isSymlink,
		}
		c.mu.Unlock()

		if isSymlink {
			return &TraversesSymlinkError{path: path}
		}
		if !isDir {
			return &NotADirectoryError{path: path}
		}
	}

	return nil
}

// Invalidate clears the cache.
func (c *SymlinkCache) Invalidate() {
	c.mu.Lock()
	c.cache = make(map[string]symlinkCacheEntry)
	c.mu.Unlock()
}

// InvalidatePath removes a specific path from the cache.
func (c *SymlinkCache) InvalidatePath(path string) {
	c.mu.Lock()
	delete(c.cache, path)
	c.mu.Unlock()
}
