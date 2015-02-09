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
	"syscall"
	"time"
)

func init() {
	if innerProcess && os.Getenv("STBLOCKPROFILE") != "" {
		profiler := pprof.Lookup("block")
		if profiler == nil {
			panic("Couldn't find block profiler")
		}
		l.Debugln("Starting block profiling")
		go saveBlockingProfiles(profiler)
	}
}

func saveBlockingProfiles(profiler *pprof.Profile) {
	runtime.SetBlockProfileRate(1)

	t0 := time.Now()
	for t := range time.NewTicker(20 * time.Second).C {
		startms := int(t.Sub(t0).Seconds() * 1000)

		fd, err := os.Create(fmt.Sprintf("block-%05d-%07d.pprof", syscall.Getpid(), startms))
		if err != nil {
			panic(err)
		}
		err = profiler.WriteTo(fd, 0)
		if err != nil {
			panic(err)
		}
		err = fd.Close()
		if err != nil {
			panic(err)
		}

	}
}
