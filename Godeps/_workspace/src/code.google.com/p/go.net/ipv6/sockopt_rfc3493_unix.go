// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd

package ipv6

import (
	"net"
	"os"
	"syscall"
)

func ipv6TrafficClass(fd int) (int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_TCLASS)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return v, nil
}

func setIPv6TrafficClass(fd, v int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_TCLASS, v))
}

func ipv6HopLimit(fd int) (int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_UNICAST_HOPS)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return v, nil
}

func setIPv6HopLimit(fd, v int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_UNICAST_HOPS, v))
}

func ipv6Checksum(fd int) (bool, int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_CHECKSUM)
	if err != nil {
		return false, 0, os.NewSyscallError("getsockopt", err)
	}
	on := true
	if v == -1 {
		on = false
	}
	return on, v, nil
}

func ipv6MulticastHopLimit(fd int) (int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_HOPS)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return v, nil
}

func setIPv6MulticastHopLimit(fd, v int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_HOPS, v))
}

func ipv6MulticastInterface(fd int) (*net.Interface, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_IF)
	if err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	if v == 0 {
		return nil, nil
	}
	ifi, err := net.InterfaceByIndex(v)
	if err != nil {
		return nil, err
	}
	return ifi, nil
}

func setIPv6MulticastInterface(fd int, ifi *net.Interface) error {
	var v int
	if ifi != nil {
		v = ifi.Index
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_IF, v))
}

func ipv6MulticastLoopback(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_LOOP)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6MulticastLoopback(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_LOOP, boolint(v)))
}

func joinIPv6Group(fd int, ifi *net.Interface, grp net.IP) error {
	mreq := syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], grp)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptIPv6Mreq(fd, ianaProtocolIPv6, syscall.IPV6_JOIN_GROUP, &mreq))
}

func leaveIPv6Group(fd int, ifi *net.Interface, grp net.IP) error {
	mreq := syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], grp)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptIPv6Mreq(fd, ianaProtocolIPv6, syscall.IPV6_LEAVE_GROUP, &mreq))
}
