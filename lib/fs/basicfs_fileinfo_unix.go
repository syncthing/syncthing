// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package fs

import (
	"os"
	"syscall"
)

func (e basicFileInfo) Mode() FileMode {
	return FileMode(e.FileInfo.Mode())
}

func (e basicFileInfo) Owner() int {
	if st, ok := e.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}

func (e basicFileInfo) Group() int {
	if st, ok := e.Sys().(*syscall.Stat_t); ok {
		return int(st.Gid)
	}
	return -1
}

// fileStat converts e to os.FileInfo that is suitable
// to be passed to os.SameFile. Non-trivial on Windows.
func (e *basicFileInfo) osFileInfo() os.FileInfo {
	return e.FileInfo
}
