// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	handle, err := windows.GetCurrentProcess()
	if err != nil {
		return fmt.Errorf("get process handle: %w", err)
	}
	defer windows.CloseHandle(handle)

	if cur, err := windows.GetPriorityClass(handle); err == nil && (cur == windows.IDLE_PRIORITY_CLASS || cur == windows.BELOW_NORMAL_PRIORITY_CLASS) {
		// We're done here.
		return nil
	}

	if err := windows.SetPriorityClass(handle, windows.BELOW_NORMAL_PRIORITY_CLASS); err != nil {
		return fmt.Errorf("set priority class: %w", err)
	}
	return nil
}
