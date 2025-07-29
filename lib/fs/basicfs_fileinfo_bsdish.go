// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build darwin || freebsd || netbsd
// +build darwin freebsd netbsd

package fs

import (
	"syscall"
	"time"
)

func (fi basicFileInfo) InodeChangeTime() time.Time {
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(0, sys.Ctimespec.Nano())
	}
	return time.Time{}
}
