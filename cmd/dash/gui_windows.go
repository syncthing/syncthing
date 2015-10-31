// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//+build windows

package main

import (
	"syscall"
	"time"
)

func init() {
	go trackCPUUsage()
}

func trackCPUUsage() {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		l.Warnln("Cannot track CPU usage:", err)
		return
	}

	var ctime, etime, ktime, utime syscall.Filetime
	err = syscall.GetProcessTimes(handle, &ctime, &etime, &ktime, &utime)
	if err != nil {
		l.Warnln("Cannot track CPU usage:", err)
		return
	}

	prevTime := ctime.Nanoseconds()
	prevUsage := ktime.Nanoseconds() + utime.Nanoseconds() // Always overflows

	for _ = range time.NewTicker(time.Second).C {
		err := syscall.GetProcessTimes(handle, &ctime, &etime, &ktime, &utime)
		if err != nil {
			continue
		}

		curTime := time.Now().UnixNano()
		timeDiff := curTime - prevTime
		curUsage := ktime.Nanoseconds() + utime.Nanoseconds()
		usageDiff := curUsage - prevUsage
		cpuUsageLock.Lock()
		copy(cpuUsagePercent[1:], cpuUsagePercent[0:])
		cpuUsagePercent[0] = 100 * float64(usageDiff) / float64(timeDiff)
		cpuUsageLock.Unlock()
		prevTime = curTime
		prevUsage = curUsage
	}
}
