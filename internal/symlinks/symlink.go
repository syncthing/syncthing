// Copyright (C) 2014 The Syncthing Authors.
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

package symlinks

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
)

var (
	Supported      = false
	ErrUnsupported = errors.New("symlinks not supported")
)

func init() {
	Supported = symlinksSupported()
}

func Read(path string) (string, uint32, error) {
	if !Supported {
		return "", 0, ErrUnsupported
	}

	var mode uint32
	stat, err := os.Stat(path)
	if err != nil {
		mode = protocol.FlagSymlinkMissingTarget
	} else if stat.IsDir() {
		mode = protocol.FlagDirectory
	}
	path, err = os.Readlink(path)

	return osutil.NormalizedFilename(path), mode, err
}

func IsSymlink(path string) (bool, error) {
	if !Supported {
		return false, ErrUnsupported
	}

	lstat, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return lstat.Mode()&os.ModeSymlink != 0, nil
}

func Create(target, source string, flags uint32) error {
	if !Supported {
		return ErrUnsupported
	}

	return os.Symlink(osutil.NativeFilename(target), source)
}

func ChangeType(path string, flags uint32) error {
	if runtime.GOOS != "windows" {
		// This is a Windows-only concept.
		return nil
	}

	if !Supported {
		return ErrUnsupported
	}

	target, cflags, err := Read(path)
	if err != nil {
		return err
	}

	// If it's the same type, nothing to do.
	if cflags&protocol.SymlinkTypeMask == flags&protocol.SymlinkTypeMask {
		return nil
	}

	// If the actual type is unknown, but the new type is file, nothing to do
	if cflags&protocol.FlagSymlinkMissingTarget != 0 && flags&protocol.FlagDirectory == 0 {
		return nil
	}

	return osutil.InWritableDir(func(path string) error {
		// It should be a symlink as well hence no need to change permissions
		// on the file.
		os.Remove(path)
		return Create(target, path, flags)
	}, path)
}

func symlinksSupported() bool {
	if runtime.GOOS != "windows" {
		// Symlinks are supported. In practice there may be deviations (FAT
		// filesystems etc), but these get handled and reported as the errors
		// they are when they happen.
		return true
	}

	// We try to create a symlink and verify that it looks like we expected.
	// Needs administrator priviledges and a version higher than XP.

	base := os.TempDir()
	path := filepath.Join(base, "syncthing-symlink-test")
	defer os.Remove(path)

	err := Create(base, path, protocol.FlagDirectory)
	if err != nil {
		return false
	}

	isLink, err := IsSymlink(path)
	if err != nil || !isLink {
		return false
	}

	target, flags, err := Read(path)
	if err != nil || osutil.NativeFilename(target) != base || flags&protocol.FlagDirectory == 0 {
		return false
	}

	return true
}
