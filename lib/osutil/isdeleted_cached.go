// Copyright (C) 2025 The Syncthing Authors & bxff
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/

package osutil

import (
	"path/filepath"

	"github.com/syncthing/syncthing/lib/fs"
)

// IsDeletedCached checks if a file has been deleted using cached directory
// listings and symlink traversal results for better performance.
func IsDeletedCached(ffs fs.Filesystem, name string, dirCache *DirExistenceCache, symlinkCache *SymlinkCache) bool {
	// Check file existence using cached directory listing
	exists, err := dirCache.FileExists(name)
	if err != nil {
		// Error reading directory - fall back to considering the file deleted
		// (this matches the behavior of the original IsDeleted)
		return true
	}
	if !exists {
		return true
	}

	// Check if the path traverses a symlink using cached results
	switch symlinkCache.TraversesSymlinkCached(filepath.Dir(name)).(type) {
	case *NotADirectoryError, *TraversesSymlinkError:
		return true
	}

	return false
}
