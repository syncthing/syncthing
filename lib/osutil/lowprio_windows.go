// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"syscall"
)

const processModeBackgroundBegin = 0x00100000

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() {
	setPriorityClass, err := syscall.GetProcAddress(kernel32, "SetPriorityClass")
	if err != nil {
		return
	}

	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)

	// error return ignored
	syscall.Syscall(uintptr(setPriorityClass), uintptr(handle), processModeBackgroundBegin, 0, 0)
}
