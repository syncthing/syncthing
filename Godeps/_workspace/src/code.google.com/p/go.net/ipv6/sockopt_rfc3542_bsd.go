// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build freebsd netbsd openbsd

package ipv6

import (
	"os"
	"syscall"
)

func ipv6PathMTU(fd int) (int, error) {
	v, err := syscall.GetsockoptIPv6MTUInfo(fd, ianaProtocolIPv6, syscall.IPV6_PATHMTU)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v.Mtu), nil
}

func ipv6ReceivePathMTU(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVPATHMTU)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePathMTU(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_RECVPATHMTU, boolint(v)))
}

func ipv6ICMPFilter(fd int) (*ICMPFilter, error) {
	v, err := syscall.GetsockoptICMPv6Filter(fd, ianaProtocolIPv6ICMP, syscall.ICMP6_FILTER)
	if err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	return &ICMPFilter{rawICMPFilter: rawICMPFilter{*v}}, nil
}

func setIPv6ICMPFilter(fd int, f *ICMPFilter) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptICMPv6Filter(fd, ianaProtocolIPv6ICMP, syscall.ICMP6_FILTER, &f.rawICMPFilter.ICMPv6Filter))
}
