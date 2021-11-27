// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	handle, err := windows.GetCurrentProcess()
	if err != nil {
		return errors.Wrap(err, "get process handle")
	}
	defer windows.CloseHandle(handle)

	err = windows.SetPriorityClass(handle, windows.BELOW_NORMAL_PRIORITY_CLASS)
	return errors.Wrap(err, "set priority class") // wraps nil as nil
}
