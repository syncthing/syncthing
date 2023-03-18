// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build linux || android
// +build linux android

package fs

import (
	"os"
	"syscall"
	"time"
)

// Lstat is like os.Lstat, except lobotomized for Android. See
// https://forum.syncthing.net/t/2395
func (*BasicFilesystem) underlyingLstat(name string) (fi os.FileInfo, err error) {
	for i := 0; i < 10; i++ { // We have to draw the line somewhere
		fi, err = os.Lstat(name)
		if err, ok := err.(*os.PathError); ok && err.Err == syscall.EINTR {
			time.Sleep(time.Duration(i+1) * time.Millisecond)
			continue
		}
		return
	}
	return
}
