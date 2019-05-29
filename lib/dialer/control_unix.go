// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !solaris,!windows

package dialer

import (
	"syscall"

	"golang.org/x/sys/unix"
)

var SupportsReusePort = false

func init() {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		l.Debugln("Failed to create a socket", err)
		return
	}
	defer func() { _ = syscall.Close(fd) }()

	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	switch {
	case err == syscall.ENOPROTOOPT || err == syscall.EINVAL:
		l.Debugln("SO_REUSEPORT not supported")
	case err != nil:
		l.Debugln("Unknown error when determining SO_REUSEPORT support", err)
	default:
		l.Debugln("SO_REUSEPORT supported")
		SupportsReusePort = true
	}
}

func ReusePortControl(network, address string, c syscall.RawConn) error {
	if !SupportsReusePort {
		return nil
	}
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}
