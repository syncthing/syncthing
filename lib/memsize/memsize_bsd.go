// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build freebsd openbsd dragonfly

package memsize

import "syscall"

func MemorySize() (int64, error) {
	s, err := syscall.SysctlUint32("hw.physmem")
	if err != nil {
		return 0, err
	}
	return int64(s), nil
}
