// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"os"
	"syscall"
)

func ipv6ReceiveTrafficClass(fd int) (bool, error) {
	return false, errNotSupported
}

func setIPv6ReceiveTrafficClass(fd int, v bool) error {
	return errNotSupported
}

func ipv6ReceiveHopLimit(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_2292HOPLIMIT)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveHopLimit(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_2292HOPLIMIT, boolint(v)))
}

func ipv6ReceivePacketInfo(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_2292PKTINFO)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePacketInfo(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_2292PKTINFO, boolint(v)))
}

func ipv6PathMTU(fd int) (int, error) {
	return 0, errNotSupported
}

func ipv6ReceivePathMTU(fd int) (bool, error) {
	return false, errNotSupported
}

func setIPv6ReceivePathMTU(fd int, v bool) error {
	return errNotSupported
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
