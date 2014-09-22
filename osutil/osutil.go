// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package osutil implements utilities for native OS support.
package osutil

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Try to keep this entire operation atomic-like. We shouldn't be doing this
// often enough that there is any contention on this lock.
var renameLock sync.Mutex

// Rename renames a file, while trying hard to succeed on various systems by
// temporarily tweaking directory permissions and removing the destination
// file when necessary. Will make sure to delete the from file if the
// operation fails, so use only for situations like committing a temp file to
// it's final location.
func Rename(from, to string) error {
	renameLock.Lock()
	defer renameLock.Unlock()

	// Make sure the destination directory is writeable
	toDir := filepath.Dir(to)
	if info, err := os.Stat(toDir); err == nil {
		os.Chmod(toDir, 0777)
		defer os.Chmod(toDir, info.Mode())
	}

	// On Windows, make sure the destination file is writeable (or we can't delete it)
	if runtime.GOOS == "windows" {
		os.Chmod(to, 0666)
		err := os.Remove(to)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// Don't leave a dangling temp file in case of rename error
	defer os.Remove(from)
	return os.Rename(from, to)
}
