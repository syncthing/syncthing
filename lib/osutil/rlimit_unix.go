// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package osutil

import (
	"runtime"
	"syscall"
)

const (
	darwinOpenMax = 10240
)

// MaximizeOpenFileLimit tries to set the resource limit RLIMIT_NOFILE (number
// of open file descriptors) to the max (hard limit), if the current (soft
// limit) is below the max. Returns the new (though possibly unchanged) limit,
// or an error if it could not be changed.
func MaximizeOpenFileLimit() (int, error) {
	// Get the current limit on number of open files.
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return 0, err
	}

	// If we're already at max, there's no need to try to raise the limit.
	if lim.Cur >= lim.Max {
		return int(lim.Cur), nil
	}

	// macOS doesn't like a soft limit greater then OPEN_MAX
	// See also: man setrlimit
	if runtime.GOOS == "darwin" && lim.Max > darwinOpenMax {
		lim.Max = darwinOpenMax
	}

	// Try to increase the limit to the max.
	oldLimit := lim.Cur
	lim.Cur = lim.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return int(oldLimit), err
	}

	// If the set succeeded, perform a new get to see what happened. We might
	// have gotten a value lower than the one in lim.Max, if lim.Max was
	// something that indicated "unlimited" (i.e. intmax).
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		// We don't really know the correct value here since Getrlimit
		// mysteriously failed after working once... Shouldn't ever happen, I
		// think.
		return 0, err
	}

	return int(lim.Cur), nil
}
