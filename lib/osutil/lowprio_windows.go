// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"syscall"

	"github.com/pkg/errors"
)

var (
	kernel32, _         = syscall.LoadLibrary("kernel32.dll")
	setPriorityClass, _ = syscall.GetProcAddress(kernel32, "SetPriorityClass")
)

const (
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms686219(v=vs.85).aspx
	aboveNormalPriorityClass   = 0x00008000
	belowNormalPriorityClass   = 0x00004000
	highPriorityClass          = 0x00000080
	idlePriorityClass          = 0x00000040
	normalPriorityClass        = 0x00000020
	processModeBackgroundBegin = 0x00100000
	processModeBackgroundEnd   = 0x00200000
	realtimePriorityClass      = 0x00000100
)

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return errors.Wrap(err, "get process handler")
	}
	defer syscall.CloseHandle(handle)

	res, _, err = syscall.Syscall(uintptr(setPriorityClass), uintptr(handle), belowNormalPriorityClass, 0, 0)
	if res != 0 {
		// "If the function succeeds, the return value is nonzero."
		return nil
	}
	return errors.Wrap(err, "set priority class") // wraps nil as nil
}
