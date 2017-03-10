// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration,benchmark,windows

package integration

import (
	"log"
	"os"
	"syscall"
	"time"
)

func ftToDuration(ft *syscall.Filetime) time.Duration {
	n := int64(ft.HighDateTime)<<32 + int64(ft.LowDateTime) // in 100-nanosecond intervals
	return time.Duration(n*100) * time.Nanosecond
}

func printUsage(name string, proc *os.ProcessState) {
	if rusage, ok := proc.SysUsage().(*syscall.Rusage); ok {
		log.Printf("%s: Utime: %s", name, ftToDuration(&rusage.UserTime))
		log.Printf("%s: Stime: %s", name, ftToDuration(&rusage.KernelTime))
	}
}
