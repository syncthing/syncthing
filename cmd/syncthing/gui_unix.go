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

//+build !windows,!solaris

package main

import (
	"syscall"
	"time"
)

func init() {
	go trackCPUUsage()
}

func trackCPUUsage() {
	var prevUsage int64
	var prevTime = time.Now().UnixNano()
	var rusage syscall.Rusage
	for _ = range time.NewTicker(time.Second).C {
		syscall.Getrusage(syscall.RUSAGE_SELF, &rusage)
		curTime := time.Now().UnixNano()
		timeDiff := curTime - prevTime
		curUsage := rusage.Utime.Nano() + rusage.Stime.Nano()
		usageDiff := curUsage - prevUsage
		cpuUsageLock.Lock()
		copy(cpuUsagePercent[1:], cpuUsagePercent[0:])
		cpuUsagePercent[0] = 100 * float64(usageDiff) / float64(timeDiff)
		cpuUsageLock.Unlock()
		prevTime = curTime
		prevUsage = curUsage
	}
}
