// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build heapprof

package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

func init() {
	go saveHeapProfiles()
}

func saveHeapProfiles() {
	runtime.MemProfileRate = 1
	var memstats, prevMemstats runtime.MemStats

	t0 := time.Now()
	for t := range time.NewTicker(250 * time.Millisecond).C {
		startms := int(t.Sub(t0).Seconds() * 1000)
		runtime.ReadMemStats(&memstats)
		if memstats.HeapInuse > prevMemstats.HeapInuse {
			fd, err := os.Create(fmt.Sprintf("heap-%05d-%07d.pprof", syscall.Getpid(), startms))
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
			prevMemstats = memstats
		}
	}
}
