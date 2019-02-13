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
	"path/filepath"
	"runtime"
	"strings"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/sync"
)

// Try to keep this entire operation atomic-like. We shouldn't be doing this
// often enough that there is any contention on this lock.
var renameLock = sync.NewMutex()

// RenameOrCopy renames a file, leaving source file intact in case of failure.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func RenameOrCopy(src, dst fs.Filesystem, from, to string) error {
	renameLock.Lock()
	defer renameLock.Unlock()

	return withPreparedTarget(dst, from, to, func() error {
		// Optimisation 1
		if src.Type() == dst.Type() && src.URI() == dst.URI() {
			return src.Rename(from, to)
		}

		// "Optimisation" 2
		// Try to find a common prefix between the two filesystems, use that as the base for the new one
		// and try a rename.
		if src.Type() == dst.Type() {
			commonPrefix := fs.CommonPrefix(src.URI(), dst.URI())
			if len(commonPrefix) > 0 {
				commonFs := fs.NewFilesystem(src.Type(), commonPrefix)
				err := commonFs.Rename(
					filepath.Join(strings.TrimPrefix(src.URI(), commonPrefix), from),
					filepath.Join(strings.TrimPrefix(dst.URI(), commonPrefix), to),
				)
				if err == nil {
					return nil
				}
			}
		}

		// Everything is sad, do a copy and delete.
		if _, err := dst.Stat(to); !fs.IsNotExist(err) {
			err := dst.Remove(to)
			if err != nil {
				return err
			}
		}

		err := copyFileContents(src, dst, from, to)
		if err != nil {
			_ = dst.Remove(to)
			return err
		}

		return withPreparedTarget(src, from, from, func() error {
			return src.Remove(from)
		})
	})
}

// Copy copies the file content from source to destination.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Copy(src, dst fs.Filesystem, from, to string) (err error) {
	return withPreparedTarget(dst, from, to, func() error {
		return copyFileContents(src, dst, from, to)
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
// destination file exists, all its contents will be replaced by the contents
// of the source file.
func copyFileContents(srcFs, dstFs fs.Filesystem, src, dst string) (err error) {
	in, err := srcFs.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := dstFs.Create(dst)
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

func IsDeleted(ffs fs.Filesystem, name string) bool {
	if _, err := ffs.Lstat(name); fs.IsNotExist(err) {
		return true
	}
	switch TraversesSymlink(ffs, filepath.Dir(name)).(type) {
	case *NotADirectoryError, *TraversesSymlinkError:
		return true
	}
	return false
}
