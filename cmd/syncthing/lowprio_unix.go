// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package main

import "syscall"

func setLowPriority() {
	// Process zero is "self", niceness value 9 is something between 0
	// (default) and 19 (worst priority). Error return ignored.
	syscall.Setpriority(syscall.PRIO_PROCESS, 0, 9)
}
