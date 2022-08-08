// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package scanner

import (
	"syscall"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

func setSyscallStatData(fi *protocol.FileInfo, stat fs.FileInfo) {
	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		fi.InodeChangeNs = sys.Ctimespec.Nano()
	}
}
