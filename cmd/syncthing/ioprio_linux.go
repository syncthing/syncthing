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
	syscall.Syscall(syscall.SYS_IOPRIO_SET,
		uintptr(IOPRIO_WHO_PROCESS), 0,
		uintptr(class)<<IOPRIO_CLASS_SHIFT|uintptr(value))
}
