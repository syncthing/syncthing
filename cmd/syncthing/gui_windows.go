// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
