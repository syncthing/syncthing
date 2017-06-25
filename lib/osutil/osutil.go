// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package osutil implements utilities for native OS support.
package osutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/sync"
)

// Try to keep this entire operation atomic-like. We shouldn't be doing this
// often enough that there is any contention on this lock.
var renameLock = sync.NewMutex()

// TryRename renames a file, leaving source file intact in case of failure.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func TryRename(filesystem fs.Filesystem, from, to string) error {
	renameLock.Lock()
	defer renameLock.Unlock()

	return withPreparedTarget(filesystem, from, to, func() error {
		return filesystem.Rename(from, to)
	})
}

// Rename moves a temporary file to it's final place.
// Will make sure to delete the from file if the operation fails, so use only
// for situations like committing a temp file to it's final location.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Rename(filesystem fs.Filesystem, from, to string) error {
	// Don't leave a dangling temp file in case of rename error
	if !(runtime.GOOS == "windows" && strings.EqualFold(from, to)) {
		defer filesystem.Remove(from)
	}
	return TryRename(filesystem, from, to)
}

// Copy copies the file content from source to destination.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Copy(filesystem fs.Filesystem, from, to string) (err error) {
	return withPreparedTarget(filesystem, from, to, func() error {
		return copyFileContents(filesystem, from, to)
	})
}

// InWritableDir calls fn(path), while making sure that the directory
// containing `path` is writable for the duration of the call.
func InWritableDir(fn func(string) error, fs fs.Filesystem, path string) error {
	dir := filepath.Dir(path)
	info, err := fs.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("Not a directory: " + path)
	}
	if info.Mode()&0200 == 0 {
		// A non-writeable directory (for this user; we assume that's the
		// relevant part). Temporarily change the mode so we can delete the
		// file or directory inside it.
		err = fs.Chmod(dir, 0755)
		if err == nil {
			defer func() {
				err = fs.Chmod(dir, info.Mode())
				if err != nil {
					// We managed to change the permission bits like a
					// millisecond ago, so it'd be bizarre if we couldn't
					// change it back.
					panic(err)
				}
			}()
		}
	}

	return fn(path)
}

// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func withPreparedTarget(filesystem fs.Filesystem, from, to string, f func() error) error {
	// Make sure the destination directory is writeable
	toDir := filepath.Dir(to)
	if info, err := filesystem.Stat(toDir); err == nil && info.IsDir() && info.Mode()&0200 == 0 {
		filesystem.Chmod(toDir, 0755)
		defer filesystem.Chmod(toDir, info.Mode())
	}

	// On Windows, make sure the destination file is writeable (or we can't delete it)
	if runtime.GOOS == "windows" {
		filesystem.Chmod(to, 0666)
		if !strings.EqualFold(from, to) {
			err := filesystem.Remove(to)
			if err != nil && !fs.IsNotExist(err) {
				return err
			}
		}
	}
	return f()
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(filesystem fs.Filesystem, src, dst string) (err error) {
	in, err := filesystem.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := filesystem.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return
}

var execExts map[string]bool

func init() {
	// PATHEXT contains a list of executable file extensions, on Windows
	pathext := filepath.SplitList(os.Getenv("PATHEXT"))
	// We want the extensions in execExts to be lower case
	execExts = make(map[string]bool, len(pathext))
	for _, ext := range pathext {
		execExts[strings.ToLower(ext)] = true
	}
}

// IsWindowsExecutable returns true if the given path has an extension that is
// in the list of executable extensions.
func IsWindowsExecutable(path string) bool {
	return execExts[strings.ToLower(filepath.Ext(path))]
}
