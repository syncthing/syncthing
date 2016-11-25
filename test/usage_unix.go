// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration,benchmark,!windows

package integration

import (
	"log"
	"os"
	"runtime"
	"syscall"
	"time"
)

func printUsage(name string, proc *os.ProcessState) {
	if rusage, ok := proc.SysUsage().(*syscall.Rusage); ok {
		log.Printf("%s: Utime: %s", name, time.Duration(rusage.Utime.Nano()))
		log.Printf("%s: Stime: %s", name, time.Duration(rusage.Stime.Nano()))
		if runtime.GOOS == "darwin" {
			// Darwin reports in bytes, Linux seems to report in KiB even
			// though the manpage says otherwise.
			rusage.Maxrss /= 1024
		}
		log.Printf("%s: MaxRSS: %d KiB", name, rusage.Maxrss)
	}
}
