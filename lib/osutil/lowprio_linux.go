// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
)

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

func ioprioSet(class ioprioClass, value int) error {
	res, _, err := syscall.Syscall(syscall.SYS_IOPRIO_SET,
		uintptr(ioprioWhoProcess), 0,
		uintptr(class)<<ioprioClassShift|uintptr(value))
	if res == 0 {
		return nil
	}
	return err
}

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	const wantNiceLevel = 9

	// Move ourselves to a new process group so that we can use the process
	// group variants of Setpriority etc to affect all of our threads in one
	// go. If this fails, bail, so that we don't affect things we shouldn't.
	// If we are already the leader of our own process group, do nothing.
	if pgid, err := syscall.Getpgid(0); err != nil {
		// This error really shouldn't happen
		return errors.Wrap(err, "get process group")
	} else if pgid != os.Getpid() {
		// We are not process group leader. Elevate!
		if err := syscall.Setpgid(0, 0); err != nil {
			return errors.Wrap(err, "set process group")
		}
	}

	if cur, _ := syscall.Getpriority(syscall.PRIO_PROCESS, 0); cur >= wantNiceLevel {
		// We're done here.
		return nil
	}

	// Process zero is "self", niceness value 9 is something between 0
	// (default) and 19 (worst priority).
	if err := syscall.Setpriority(syscall.PRIO_PGRP, 0, wantNiceLevel); err != nil {
		return errors.Wrap(err, "set niceness")
	}

	// Best effort, somewhere to the end of the scale (0 through 7 being the
	// range).
	err := ioprioSet(ioprioClassBE, 5)
	return errors.Wrap(err, "set I/O priority") // wraps nil as nil
}
