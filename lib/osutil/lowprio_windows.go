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

const (
	// https://docs.microsoft.com/windows/win32/api/processthreadsapi/nf-processthreadsapi-setpriorityclass
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
	modkernel32 := syscall.NewLazyDLL("kernel32.dll")
	setPriorityClass := modkernel32.NewProc("SetPriorityClass")

	if err := setPriorityClass.Find(); err != nil {
		return errors.Wrap(err, "find proc")
	}

	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return errors.Wrap(err, "get process handler")
	}
	defer syscall.CloseHandle(handle)

	res, _, err := setPriorityClass.Call(uintptr(handle), belowNormalPriorityClass)
	if res != 0 {
		// "If the function succeeds, the return value is nonzero."
		return nil
	}
	return errors.Wrap(err, "set priority class") // wraps nil as nil
}
