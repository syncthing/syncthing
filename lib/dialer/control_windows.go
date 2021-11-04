// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package dialer

import (
	"syscall"
)

var SupportsReusePort = true

func ReusePortControl(_, _ string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		// On Windows, SO_REUSEADDR is equivalent to SO_REUSEPORT on Linux.
		opErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		l.Debugln("ReusePortControl", err)
		return err
	}
	if opErr != nil {
		l.Debugln("ReusePortControl", opErr)
	}
	return opErr
}
