// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil

import (
	"os"
	"runtime"
)

func SyncFile(path string) error {
	flag := 0
	if runtime.GOOS == "windows" {
		flag = os.O_WRONLY
	}
	fd, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return err
	}
	defer fd.Close()
	// MacOS and Windows do not flush the disk cache
	return fd.Sync()
}

func SyncDir(path string) error {
	if runtime.GOOS == "windows" {
		// not supported by Windows
		return nil
	}
	return SyncFile(path)
}
