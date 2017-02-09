// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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

func (BasicFilesystem) CreateSymlink(name, target string) error {
	return os.Symlink(target, name)
}

func (BasicFilesystem) ReadSymlink(path string) (string, error) {
	return os.Readlink(path)
}
