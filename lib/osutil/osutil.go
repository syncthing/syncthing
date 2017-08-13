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
func TryRename(from fs.Filesystem, fromPath string, to fs.Filesystem, toPath string) error {
	renameLock.Lock()
	defer renameLock.Unlock()

	return withPreparedTarget(from, fromPath, to, toPath, func() error {
		// Optimization
		if from == to || to == nil || (from.Type() == to.Type() && from.URI() == to.URI()) {
			return from.Rename(fromPath, toPath)
		}

		// See if one is a prefix of the other filesystem.
		if from.Type() == to.Type() {
			fromUri := from.URI()
			toUri := to.URI()
			if strings.HasPrefix(toUri, fromUri) {
				newToPath := filepath.Join(strings.TrimPrefix(toUri, fromUri), toPath)
				if err := from.Rename(fromPath, newToPath); err == nil {
					return err
				}
			} else if strings.HasPrefix(fromPath, toPath) {
				newFromPath := filepath.Join(strings.TrimPrefix(fromUri, toUri), fromPath)
				if err := to.Rename(newFromPath, toPath); err == nil {
					return err
				}
			} else {
				shorter := fromUri
				if len(shorter) > len(toUri) {
					shorter = toUri
				}
				prefix := ""
				for i := range shorter {
					if fromUri[i] == toUri[i] {
						prefix += string(fromUri[i])
					} else {
						break
					}
				}
				if prefix != "" {
					commonFs := fs.NewFilesystem(from.Type(), prefix)
					if err := commonFs.Rename(strings.TrimPrefix(filepath.Join(fromUri, fromPath), prefix), strings.TrimPrefix(filepath.Join(toUri, toPath), prefix)); err == nil {
						return nil
					}

				}
			}
		}

		return Copy(from, fromPath, to, toPath)
	})
}

// Rename moves a temporary file to it's final place.
// Will make sure to delete the from file if the operation fails, so use only
// for situations like committing a temp file to it's final location.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Rename(from fs.Filesystem, fromPath string, to fs.Filesystem, toPath string) error {
	// Don't leave a dangling temp file in case of rename error
	if !(runtime.GOOS == "windows" && strings.EqualFold(fromPath, toPath)) {
		defer from.Remove(fromPath)
	}
	return TryRename(from, fromPath, to, toPath)
}

// Copy copies the file content from source to destination.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Copy(from fs.Filesystem, fromPath string, to fs.Filesystem, toPath string) (err error) {
	return withPreparedTarget(from, fromPath, to, toPath, func() error {
		return copyFileContents(from, fromPath, to, toPath)
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
func withPreparedTarget(from fs.Filesystem, fromPath string, to fs.Filesystem, toPath string, f func() error) error {
	// Make sure the destination directory is writeable
	toDir := filepath.Dir(toPath)
	if info, err := to.Stat(toDir); err == nil && info.IsDir() && info.Mode()&0200 == 0 {
		to.Chmod(toDir, 0755)
		defer to.Chmod(toDir, info.Mode())
	}

	// On Windows, make sure the destination file is writeable (or we can't delete it)
	if runtime.GOOS == "windows" {
		to.Chmod(toPath, 0666)
		if !strings.EqualFold(fromPath, toPath) {
			err := to.Remove(toPath)
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
func copyFileContents(src fs.Filesystem, srcPath string, dst fs.Filesystem, dstPath string) (err error) {
	in, err := src.Open(srcPath)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := dst.Create(dstPath)
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
