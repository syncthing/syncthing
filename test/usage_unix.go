// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build integration && benchmark && !windows
// +build integration,benchmark,!windows

package integration

import (
	"log"
	"os"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

func printUsage(name string, proc *os.ProcessState, total int64) {
	if rusage, ok := proc.SysUsage().(*syscall.Rusage); ok {
		mib := total / 1024 / 1024
		log.Printf("%s: Utime: %s / MiB", name, time.Duration(rusage.Utime.Nano()/mib))
		log.Printf("%s: Stime: %s / MiB", name, time.Duration(rusage.Stime.Nano()/mib))
		if build.IsDarwin {
			// Darwin reports in bytes, Linux seems to report in KiB even
			// though the manpage says otherwise.
			rusage.Maxrss /= 1024
		}
		log.Printf("%s: MaxRSS: %d KiB", name, rusage.Maxrss)
	}
}
