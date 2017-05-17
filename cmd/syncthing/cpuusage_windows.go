// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//+build windows

package main

import "syscall"
import "time"

func cpuUsage() time.Duration {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(handle)

	var ctime, etime, ktime, utime syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &ctime, &etime, &ktime, &utime); err != nil {
		return 0
	}

	return time.Duration(ktime.Nanoseconds() + utime.Nanoseconds())
}
