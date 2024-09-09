// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build (!windows && !linux && !ios) || android
// +build !windows,!linux,!ios android

package osutil

import (
	"fmt"
	"syscall"
)

// SetLowPriority lowers the process CPU scheduling priority, and possibly
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	// Process zero is "self", niceness value 9 is something between 0
	// (default) and 19 (worst priority).
	const (
		pidSelf       = 0
		wantNiceLevel = 9
	)

	if cur, err := syscall.Getpriority(syscall.PRIO_PROCESS, pidSelf); err == nil && cur >= wantNiceLevel {
		// We're done here.
		return nil
	}

	if err := syscall.Setpriority(syscall.PRIO_PROCESS, pidSelf, wantNiceLevel); err != nil {
		return fmt.Errorf("set niceness: %w", err)
	}
	return nil
}
