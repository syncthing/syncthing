// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !solaris && !windows
// +build !solaris,!windows

package dialer

import (
	"log/slog"
	"syscall"

	"golang.org/x/sys/unix"
)

var SupportsReusePort = false

func init() {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_IP)
	if err != nil {
		l.Debugln("Failed to create a socket", err)
		return
	}
	defer func() { _ = unix.Close(fd) }()

	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	switch {
	case err == unix.ENOPROTOOPT || err == unix.EINVAL:
		slog.Debug("SO_REUSEPORT not supported")
	case err != nil:
		l.Debugln("Unknown error when determining SO_REUSEPORT support", err)
	default:
		slog.Debug("SO_REUSEPORT supported")
		SupportsReusePort = true
	}
}

func ReusePortControl(_, _ string, c syscall.RawConn) error {
	if !SupportsReusePort {
		return nil
	}
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}
