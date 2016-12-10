// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package symlinks

import (
	"os"

	"github.com/syncthing/syncthing/lib/osutil"
)

var (
	Supported = true
)

func Read(path string) (string, TargetType, error) {
	tt := TargetUnknown
	if stat, err := os.Stat(path); err == nil {
		if stat.IsDir() {
			tt = TargetDirectory
		} else {
			tt = TargetFile
		}
	}
	path, err := os.Readlink(path)

	return osutil.NormalizedFilename(path), tt, err
}

func Create(source, target string, tt TargetType) error {
	return os.Symlink(osutil.NativeFilename(target), source)
}
