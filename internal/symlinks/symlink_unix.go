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

// +build !windows

package symlinks

import (
	"os"

	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
)

var (
	Supported = true
)

func Read(path string) (string, uint32, error) {
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
	lstat, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return lstat.Mode()&os.ModeSymlink != 0, nil
}

func Create(source, target string, flags uint32) error {
	return os.Symlink(osutil.NativeFilename(target), source)
}

func ChangeType(path string, flags uint32) error {
	return nil
}
