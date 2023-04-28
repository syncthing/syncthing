// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !solaris && !windows
// +build !solaris,!windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/netutil"
)

func startPerfStats() {
	go savePerfStats(fmt.Sprintf("perfstats-%d.csv", syscall.Getpid()))
}

func savePerfStats(file string) {
	fd, err := os.Create(file)
	if err != nil {
		panic(err)
	}

	var prevUsage int64
	var prevTime int64
	var rusage syscall.Rusage
	var memstats runtime.MemStats
	var prevIn, prevOut int64

	t0 := time.Now()
	for t := range time.NewTicker(250 * time.Millisecond).C {
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
			continue
		}

		curTime := time.Now().UnixNano()
		timeDiff := curTime - prevTime
		curUsage := rusage.Utime.Nano() + rusage.Stime.Nano()
		usageDiff := curUsage - prevUsage
		cpuUsagePercent := 100 * float64(usageDiff) / float64(timeDiff)
		prevTime = curTime
		prevUsage = curUsage
		cnt := netutil.RootCounter()
		in, out := cnt.BytesRead(), cnt.BytesWritten()
		var inRate, outRate float64
		if timeDiff > 0 {
			inRate = float64(in-prevIn) / (float64(timeDiff) / 1e9)    // bytes per second
			outRate = float64(out-prevOut) / (float64(timeDiff) / 1e9) // bytes per second
		}
		prevIn, prevOut = in, out

		runtime.ReadMemStats(&memstats)

		startms := int(t.Sub(t0).Seconds() * 1000)

		fmt.Fprintf(fd, "%d\t%f\t%d\t%d\t%.0f\t%.0f\n", startms, cpuUsagePercent, memstats.Alloc, memstats.Sys-memstats.HeapReleased, inRate, outRate)
	}
}
