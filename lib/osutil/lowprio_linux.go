// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !android
// +build !android

package osutil

import (
	"fmt"
	"os"
	"syscall"
)

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	// Process zero is "self", niceness value 9 is something between 0
	// (default) and 19 (worst priority). But then, this is Linux, so of
	// course we get this to take care of as well:
	//
	// "C library/kernel differences
	//
	// Within  the  kernel,  nice  values are actually represented using the
	// range 40..1 (since negative numbers are error codes) and  these  are
	// the  values employed  by  the  setpriority() and getpriority() system
	// calls.  The glibc wrapper functions for these system calls handle the
	// translations  between the user-land and kernel representations of the
	// nice value according to the formula unice = 20 - knice. (Thus, the
	// kernel's 40..1 range corresponds to the range -20..19 as seen by user
	// space.)"

	const (
		pidSelf       = 0
		wantNiceLevel = 20 - 9
	)

	// Remember Linux kernel nice levels are upside down.
	if cur, err := syscall.Getpriority(syscall.PRIO_PROCESS, 0); err == nil && cur <= wantNiceLevel {
		// We're done here.
		return nil
	}

	// Move ourselves to a new process group so that we can use the process
	// group variants of Setpriority etc to affect all of our threads in one
	// go. If this fails, bail, so that we don't affect things we shouldn't.
	// If we are already the leader of our own process group, do nothing.
	//
	// Oh and this is because Linux doesn't follow the POSIX threading model
	// where setting the niceness of the process would actually set the
	// niceness of the process, instead it just affects the current thread
	// so we need this workaround...
	if pgid, err := syscall.Getpgid(pidSelf); err != nil {
		// This error really shouldn't happen
		return fmt.Errorf("get process group: %w", err)
	} else if pgid != os.Getpid() {
		// We are not process group leader. Elevate!
		if err := syscall.Setpgid(pidSelf, 0); err != nil {
			return fmt.Errorf("set process group: %w", err)
		}
	}

	// For any new process, the default is to be assigned the IOPRIO_CLASS_BE
	// scheduling class. This class directly maps the BE prio level to the
	// niceness of a process, determined as: io_nice = (cpu_nice + 20) / 5.
	// For example, a niceness of 11 results in an I/O priority of B6.
	// https://www.kernel.org/doc/Documentation/block/ioprio.txt
	if err := syscall.Setpriority(syscall.PRIO_PGRP, pidSelf, wantNiceLevel); err != nil {
		return fmt.Errorf("set niceness: %w", err)
	}
	return nil
}
