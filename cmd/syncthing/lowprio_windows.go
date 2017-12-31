// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"syscall"
)

var setPriorityClass, _ = syscall.GetProcAddress(kernel32, "SetPriorityClass")

const PROCESS_MODE_BACKGROUND_BEGIN = 0x00100000

func setLowPriority() {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)

	// error return ignored
	syscall.Syscall(uintptr(setPriorityClass), uintptr(handle), PROCESS_MODE_BACKGROUND_BEGIN, 0, 0)
}
