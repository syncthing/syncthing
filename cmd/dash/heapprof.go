// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"syscall"
	"time"
)

func init() {
	if innerProcess && os.Getenv("STHEAPPROFILE") != "" {
		rate := 1
		if i, err := strconv.Atoi(os.Getenv("STHEAPPROFILE")); err == nil {
			rate = i
		}
		l.Debugln("Starting heap profiling")
		go saveHeapProfiles(rate)
	}
}

func saveHeapProfiles(rate int) {
	runtime.MemProfileRate = rate
	var memstats, prevMemstats runtime.MemStats

	name := fmt.Sprintf("heap-%05d.pprof", syscall.Getpid())
	for {
		runtime.ReadMemStats(&memstats)

		if memstats.HeapInuse > prevMemstats.HeapInuse {
			fd, err := os.Create(name + ".tmp")
			if err != nil {
				panic(err)
			}
			err = pprof.WriteHeapProfile(fd)
			if err != nil {
				panic(err)
			}
			err = fd.Close()
			if err != nil {
				panic(err)
			}

			_ = os.Remove(name) // Error deliberately ignored
			err = os.Rename(name+".tmp", name)
			if err != nil {
				panic(err)
			}

			prevMemstats = memstats
		}

		time.Sleep(250 * time.Millisecond)
	}
}
