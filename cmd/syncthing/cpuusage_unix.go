// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//+build !windows,!solaris

package main

import "syscall"
import "time"

func cpuUsage() time.Duration {
	var rusage syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &rusage)
	return time.Duration(rusage.Utime.Nano() + rusage.Stime.Nano())
}
