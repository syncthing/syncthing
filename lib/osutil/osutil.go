// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package osutil implements utilities for native OS support.
package osutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/calmh/du"
	"github.com/syncthing/syncthing/lib/sync"
)

var ErrNoHome = errors.New("No home directory found - set $HOME (or the platform equivalent).")

// Try to keep this entire operation atomic-like. We shouldn't be doing this
// often enough that there is any contention on this lock.
var renameLock = sync.NewMutex()

// TryRename renames a file, leaving source file intact in case of failure.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func TryRename(from, to string) error {
	renameLock.Lock()
	defer renameLock.Unlock()

	return withPreparedTarget(from, to, func() error {
		return os.Rename(from, to)
	})
}

// Rename moves a temporary file to it's final place.
// Will make sure to delete the from file if the operation fails, so use only
// for situations like committing a temp file to it's final location.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Rename(from, to string) error {
	// Don't leave a dangling temp file in case of rename error
	if !(runtime.GOOS == "windows" && strings.EqualFold(from, to)) {
		defer os.Remove(from)
	}
	return TryRename(from, to)
}

// Copy copies the file content from source to destination.
// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func Copy(from, to string) (err error) {
	return withPreparedTarget(from, to, func() error {
		return copyFileContents(from, to)
	})
}

// InWritableDir calls fn(path), while making sure that the directory
// containing `path` is writable for the duration of the call.
func InWritableDir(fn func(string) error, path string) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
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
		err = os.Chmod(dir, 0755)
		if err == nil {
			defer func() {
				err = os.Chmod(dir, info.Mode())
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

// Remove removes the given path. On Windows, removes the read-only attribute
// from the target prior to deletion.
func Remove(path string) error {
	if runtime.GOOS == "windows" {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Mode()&0200 == 0 {
			os.Chmod(path, 0700)
		}
	}
	return os.Remove(path)
}

func ExpandTilde(path string) (string, error) {
	if path == "~" {
		return getHomeDir()
	}

	path = filepath.FromSlash(path)
	if !strings.HasPrefix(path, fmt.Sprintf("~%c", os.PathSeparator)) {
		return path, nil
	}

	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}

func getHomeDir() (string, error) {
	var home string

	switch runtime.GOOS {
	case "windows":
		home = filepath.Join(os.Getenv("HomeDrive"), os.Getenv("HomePath"))
		if home == "" {
			home = os.Getenv("UserProfile")
		}
	default:
		home = os.Getenv("HOME")
	}

	if home == "" {
		return "", ErrNoHome
	}

	return home, nil
}

// Tries hard to succeed on various systems by temporarily tweaking directory
// permissions and removing the destination file when necessary.
func withPreparedTarget(from, to string, f func() error) error {
	// Make sure the destination directory is writeable
	toDir := filepath.Dir(to)
	if info, err := os.Stat(toDir); err == nil && info.IsDir() && info.Mode()&0200 == 0 {
		os.Chmod(toDir, 0755)
		defer os.Chmod(toDir, info.Mode())
	}

	// On Windows, make sure the destination file is writeable (or we can't delete it)
	if runtime.GOOS == "windows" {
		os.Chmod(to, 0666)
		if !strings.EqualFold(from, to) {
			err := os.Remove(to)
			if err != nil && !os.IsNotExist(err) {
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
func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
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

func DiskFreeBytes(path string) (free int64, err error) {
	u, err := du.Get(path)
	return u.FreeBytes, err
}

func DiskFreePercentage(path string) (freePct float64, err error) {
	u, err := du.Get(path)
	return (float64(u.FreeBytes) / float64(u.TotalBytes)) * 100, err
}

// SetTCPOptions sets syncthings default TCP options on a TCP connection
func SetTCPOptions(conn *net.TCPConn) error {
	var err error
	if err = conn.SetLinger(0); err != nil {
		return err
	}
	if err = conn.SetNoDelay(false); err != nil {
		return err
	}
	if err = conn.SetKeepAlivePeriod(60 * time.Second); err != nil {
		return err
	}
	if err = conn.SetKeepAlive(true); err != nil {
		return err
	}
	return nil
}

// The CachedCaseSensitiveStat provides an Lstat() method similar to
// os.Lstat(), but that is always case sensitive regardless of underlying file
// system semantics. The "Cached" part refers to the fact that it lists the
// contents of a directory the first time it's needed and then retains this
// information for the duration. It's expected that instances of this type are
// fairly short lived.
//
// There's some song and dance to check directories that are parents to the
// checked path as well, that is we want to catch the situation that someone
// calls Lstat("foo/BAR/baz") when the actual path is "foo/bar/baz" and return
// NotExist appropriately. But we don't want to do this check too high up, as
// the user may have told us the folder path is ~/Sync while it is actually
// ~/sync and this *should* work properly... Sigh. Hence the "base" parameter.
type CachedCaseSensitiveStat struct {
	base    string                   // base directory, we should not check stuff above this
	results map[string][]os.FileInfo // directory path => list of children
}

func NewCachedCaseSensitiveStat(base string) *CachedCaseSensitiveStat {
	return &CachedCaseSensitiveStat{
		base:    strings.ToLower(base),
		results: make(map[string][]os.FileInfo),
	}
}

func (c *CachedCaseSensitiveStat) Lstat(name string) (os.FileInfo, error) {
	dir := filepath.Dir(name)
	base := filepath.Base(name)

	if !strings.HasPrefix(strings.ToLower(dir), c.base) {
		// We only validate things within the base directory, which we need to
		// compare case insensitively against.
		return nil, os.ErrInvalid
	}

	// If we don't already have a list of directory entries for this
	// directory, try to list it. Return error if this fails.
	l, ok := c.results[dir]
	if !ok {
		if len(dir) > len(c.base) {
			// We are checking in a subdirectory of base. Must make sure *it*
			// exists with the specified casing, up to the base directory.
			if _, err := c.Lstat(dir); err != nil {
				return nil, err
			}
		}

		fd, err := os.Open(dir)
		if err != nil {
			return nil, err
		}
		defer fd.Close()

		l, err = fd.Readdir(-1)
		if err != nil {
			return nil, err
		}

		sort.Sort(fileInfoList(l))
		c.results[dir] = l
	}

	// Get the index of the first entry with name >= base using binary search.
	idx := sort.Search(len(l), func(i int) bool {
		return l[i].Name() >= base
	})

	if idx >= len(l) || l[idx].Name() != base {
		// The search didn't find any such entry
		return nil, os.ErrNotExist
	}

	return l[idx], nil
}

type fileInfoList []os.FileInfo

func (l fileInfoList) Len() int {
	return len(l)
}
func (l fileInfoList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l fileInfoList) Less(a, b int) bool {
	return l[a].Name() < l[b].Name()
}
