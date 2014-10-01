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

// +build !solaris,!windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/internal/protocol"
)

func init() {
	if innerProcess && os.Getenv("STPERFSTATS") != "" {
		go savePerfStats(fmt.Sprintf("perfstats-%d.csv", syscall.Getpid()))
	}
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
	var prevIn, prevOut uint64

	t0 := time.Now()
	for t := range time.NewTicker(250 * time.Millisecond).C {
		syscall.Getrusage(syscall.RUSAGE_SELF, &rusage)
		curTime := time.Now().UnixNano()
		timeDiff := curTime - prevTime
		curUsage := rusage.Utime.Nano() + rusage.Stime.Nano()
		usageDiff := curUsage - prevUsage
		cpuUsagePercent := 100 * float64(usageDiff) / float64(timeDiff)
		prevTime = curTime
		prevUsage = curUsage
		in, out := protocol.TotalInOut()
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
