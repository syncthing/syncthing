// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import "syscall"

const IOPRIO_CLASS_SHIFT = 13

type ioprioClass int

const (
	IOPRIO_CLASS_RT ioprioClass = iota + 1
	IOPRIO_CLASS_BE
	IOPRIO_CLASS_IDLE
)

const (
	IOPRIO_WHO_PROCESS = iota + 1
	IOPRIO_WHO_PGRP
	IOPRIO_WHO_USER
)

func ioprioSet(class ioprioClass, value int) {
	// error return ignored
	syscall.Syscall(syscall.SYS_IOPRIO_SET,
		uintptr(IOPRIO_WHO_PROCESS), 0,
		uintptr(class)<<IOPRIO_CLASS_SHIFT|uintptr(value))
}

func setLowPriority() {
	// Process zero is "self", niceness value 9 is something between 0
	// (default) and 19 (worst priority). Error return ignored.
	syscall.Setpriority(syscall.PRIO_PROCESS, 0, 9)

	// Best effort, somewhere to the end of the scale (0 through 7 being the
	// range). Error return ignored.
	ioprioSet(IOPRIO_CLASS_BE, 5)
}
