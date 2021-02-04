// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

func startHeapProfiler() {
	l.Debugln("Starting heap profiling")
	go func() {
		err := saveHeapProfiles(1) // Only returns on error
		l.Warnln("Heap profiler failed:", err)
		panic("Heap profiler failed")
	}()
}

func saveHeapProfiles(rate int) error {
	runtime.MemProfileRate = rate
	var memstats, prevMemstats runtime.MemStats

	name := fmt.Sprintf("heap-%05d.pprof", syscall.Getpid())
	for {
		runtime.ReadMemStats(&memstats)

		if memstats.HeapInuse > prevMemstats.HeapInuse {
			fd, err := os.Create(name + ".tmp")
			if err != nil {
				return err
			}
			err = pprof.WriteHeapProfile(fd)
			if err != nil {
				return err
			}
			err = fd.Close()
			if err != nil {
				return err
			}

			os.Remove(name) // Error deliberately ignored
			err = os.Rename(name+".tmp", name)
			if err != nil {
				return err
			}

			prevMemstats = memstats
		}

		time.Sleep(250 * time.Millisecond)
	}
}
