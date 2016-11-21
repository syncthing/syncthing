// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import "os"

var symlinksSupported = true

func DisableSymlinks() {
	symlinksSupported = false
}

func (BasicFilesystem) SymlinksSupported() bool {
	return symlinksSupported
}

func (BasicFilesystem) CreateSymlink(name, target string, _ LinkTargetType) error {
	return os.Symlink(target, name)
}

func (BasicFilesystem) ChangeSymlinkType(_ string, _ LinkTargetType) error {
	return nil
}

func (BasicFilesystem) ReadSymlink(path string) (string, LinkTargetType, error) {
	tt := LinkTargetUnknown
	if stat, err := os.Stat(path); err == nil {
		if stat.IsDir() {
			tt = LinkTargetDirectory
		} else {
			tt = LinkTargetFile
		}
	}

	path, err := os.Readlink(path)
	return path, tt, err
}
