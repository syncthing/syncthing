// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import "syscall"

const ioprioClassShift = 13

type ioprioClass int

const (
	ioprioClassRT ioprioClass = iota + 1
	ioprioClassBE
	ioprioClassIdle
)

const (
	ioprioWhoProcess = iota + 1
	ioprioWhoPGRP
	ioprioWhoUser
)

func ioprioSet(class ioprioClass, value int) {
	// error return ignored
	syscall.Syscall(syscall.SYS_IOPRIO_SET,
		uintptr(ioprioWhoProcess), 0,
		uintptr(class)<<ioprioClassShift|uintptr(value))
}

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() {
	// Process zero is "self", niceness value 9 is something between 0
	// (default) and 19 (worst priority). Error return ignored.
	syscall.Setpriority(syscall.PRIO_Process, 0, 9)

	// Best effort, somewhere to the end of the scale (0 through 7 being the
	// range). Error return ignored.
	ioprioSet(ioprioClassBE, 5)
}
