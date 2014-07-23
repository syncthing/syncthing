// +build perfstats

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
)

func init() {
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

		runtime.ReadMemStats(&memstats)

		startms := int(t.Sub(t0).Seconds() * 1000)

		fmt.Fprintf(fd, "%d\t%f\t%d\t%d\n", startms, cpuUsagePercent, memstats.Alloc, memstats.Sys)
	}
}
