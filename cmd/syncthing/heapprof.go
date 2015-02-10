// Copyright (C) 2014 The Syncthing Authors.
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
