// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build aix || dragonfly || linux || openbsd || solaris || illumos
// +build aix dragonfly linux openbsd solaris illumos

package fs

import (
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

func (fi basicFileInfo) InodeChangeTime() time.Time {
	// On Android, mtime and inode-change-time fluctuate, which can cause
	// conflicts even when nothing has been modified on the device itself.
	// Ref: https://forum.syncthing.net/t/keep-getting-conflicts-generated-on-android-device-for-files-modified-only-on-a-desktop-pc/19060
	if build.IsAndroid { 
		return time.Time{}
	}
	if sys, ok := fi.FileInfo.Sys().(*syscall.Stat_t); ok {
		return time.Unix(0, sys.Ctim.Nano())
	}
	return time.Time{}
}
