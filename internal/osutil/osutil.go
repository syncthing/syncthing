// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

// Package osutil implements utilities for native OS support.
package osutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var ErrNoHome = errors.New("No home directory found - set $HOME (or the platform equivalent).")

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

// InWritableDir calls fn(path), while making sure that the directory
// containing `path` is writable for the duration of the call.
func InWritableDir(fn func(string) error, path string) error {
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err == nil && info.IsDir() && info.Mode()&04 == 0 {
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
