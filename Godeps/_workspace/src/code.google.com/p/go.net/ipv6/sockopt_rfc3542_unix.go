// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build freebsd linux netbsd openbsd

package ipv6

import (
	"os"
	"syscall"
)

func ipv6ReceiveTrafficClass(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVTCLASS)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveTrafficClass(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVTCLASS, boolint(v)))
}

func ipv6ReceiveHopLimit(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVHOPLIMIT)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveHopLimit(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVHOPLIMIT, boolint(v)))
}

func ipv6ReceivePacketInfo(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVPKTINFO)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePacketInfo(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVPKTINFO, boolint(v)))
}
