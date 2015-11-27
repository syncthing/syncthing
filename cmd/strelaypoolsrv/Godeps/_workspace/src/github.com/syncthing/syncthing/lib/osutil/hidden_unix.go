// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package osutil

func HideFile(path string) error {
	return nil
}

func ShowFile(path string) error {
	return nil
}

func HideConsole() {}
