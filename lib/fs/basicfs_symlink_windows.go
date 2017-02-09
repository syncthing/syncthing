// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package fs

import "errors"

var errNotSupported = errors.New("symlinks not supported")

func DisableSymlinks() {}

func (BasicFilesystem) SymlinksSupported() bool {
	return false
}

func (BasicFilesystem) ReadSymlink(path string) (string, error) {
	return "", errNotSupported
}

func (BasicFilesystem) CreateSymlink(path, target string) error {
	return errNotSupported
}
