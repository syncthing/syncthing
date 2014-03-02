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
	for {
		time.Sleep(10 * time.Second)
		syscall.Getrusage(syscall.RUSAGE_SELF, &rusage)
		curTime := time.Now().UnixNano()
		timeDiff := curTime - prevTime
		curUsage := rusage.Utime.Nano() + rusage.Stime.Nano()
		usageDiff := curUsage - prevUsage
		cpuUsageLock.Lock()
		cpuUsagePercent = 100 * float64(usageDiff) / float64(timeDiff)
		cpuUsageLock.Unlock()
		prevTime = curTime
		prevUsage = curUsage
	}
}
