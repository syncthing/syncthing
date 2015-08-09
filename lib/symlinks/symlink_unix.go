// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package symlinks

import (
	"os"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/osutil"
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

func Create(source, target string, flags uint32) error {
	return os.Symlink(osutil.NativeFilename(target), source)
}

func ChangeType(path string, flags uint32) error {
	return nil
}
