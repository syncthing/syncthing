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

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/exp/constraints"
)

func startPerfStats() {
	go savePerfStats(fmt.Sprintf("perfstats-%d.csv", syscall.Getpid()))
}

func savePerfStats(file string) {
	fd, err := os.Create(file)
	if err != nil {
		panic(err)
	}

	var prevTime time.Time
	var curRus, prevRus syscall.Rusage
	var curMem, prevMem runtime.MemStats
	var prevIn, prevOut int64

	t0 := time.Now()
	syscall.Getrusage(syscall.RUSAGE_SELF, &prevRus)
	runtime.ReadMemStats(&prevMem)

	fmt.Fprintf(fd, "TIME_S\tCPU_S\tHEAP_KIB\tRSS_KIB\tNETIN_KBPS\tNETOUT_KBPS\tDBSIZE_KIB\n")

	for t := range time.NewTicker(250 * time.Millisecond).C {
		syscall.Getrusage(syscall.RUSAGE_SELF, &curRus)
		runtime.ReadMemStats(&curMem)
		in, out := protocol.TotalInOut()
		timeDiff := t.Sub(prevTime)

		rss := curRus.Maxrss
		if build.IsDarwin {
			rss /= 1024
		}

		fmt.Fprintf(fd, "%.03f\t%f\t%d\t%d\t%.0f\t%.0f\t%d\n",
			t.Sub(t0).Seconds(),
			rate(cpusec(&prevRus), cpusec(&curRus), timeDiff, 1),
			(curMem.Sys-curMem.HeapReleased)/1024,
			rss,
			rate(prevIn, in, timeDiff, 1e3),
			rate(prevOut, out, timeDiff, 1e3),
			osutil.DirSize(locations.Get(locations.Database))/1024,
		)

		prevTime = t
		prevRus = curRus
		prevMem = curMem
		prevIn, prevOut = in, out
	}
}

func cpusec(r *syscall.Rusage) float64 {
	return float64(r.Utime.Nano()+r.Stime.Nano()) / float64(time.Second)
}

type number interface {
	constraints.Float | constraints.Integer
}

func rate[T number](prev, cur T, d time.Duration, div float64) float64 {
	diff := cur - prev
	rate := float64(diff) / d.Seconds() / div
	return rate
}
