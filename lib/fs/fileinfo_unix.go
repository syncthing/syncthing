// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import "syscall"

func (e fsFileInfo) Owner() int {
	if st, ok := e.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}

func (e fsFileInfo) Group() int {
	if st, ok := e.Sys().(*syscall.Stat_t); ok {
		return int(st.Gid)
	}
	return -1
}
