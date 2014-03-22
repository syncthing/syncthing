// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

func ipv6TrafficClass(fd syscall.Handle) (int, error) {
	// TODO(mikio): Implement this
	return 0, syscall.EWINDOWS
}

func setIPv6TrafficClass(fd syscall.Handle, v int) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}

func ipv6HopLimit(fd syscall.Handle) (int, error) {
	var v int32
	l := int32(4)
	if err := syscall.Getsockopt(fd, ianaProtocolIPv6, syscall.IPV6_UNICAST_HOPS, (*byte)(unsafe.Pointer(&v)), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v), nil
}

func setIPv6HopLimit(fd syscall.Handle, v int) error {
	vv := int32(v)
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd, ianaProtocolIPv6, syscall.IPV6_UNICAST_HOPS, (*byte)(unsafe.Pointer(&vv)), 4))
}

func ipv6Checksum(fd syscall.Handle) (bool, int, error) {
	// TODO(mikio): Implement this
	return false, 0, syscall.EWINDOWS
}

func ipv6MulticastHopLimit(fd syscall.Handle) (int, error) {
	var v int32
	l := int32(4)
	if err := syscall.Getsockopt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_HOPS, (*byte)(unsafe.Pointer(&v)), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v), nil
}

func setIPv6MulticastHopLimit(fd syscall.Handle, v int) error {
	vv := int32(v)
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_HOPS, (*byte)(unsafe.Pointer(&vv)), 4))
}

func ipv6MulticastInterface(fd syscall.Handle) (*net.Interface, error) {
	var v int32
	l := int32(4)
	if err := syscall.Getsockopt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_IF, (*byte)(unsafe.Pointer(&v)), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	if v == 0 {
		return nil, nil
	}
	ifi, err := net.InterfaceByIndex(int(v))
	if err != nil {
		return nil, err
	}
	return ifi, nil
}

func setIPv6MulticastInterface(fd syscall.Handle, ifi *net.Interface) error {
	var v int32
	if ifi != nil {
		v = int32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_IF, (*byte)(unsafe.Pointer(&v)), 4))
}

func ipv6MulticastLoopback(fd syscall.Handle) (bool, error) {
	var v int32
	l := int32(4)
	if err := syscall.Getsockopt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_LOOP, (*byte)(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6MulticastLoopback(fd syscall.Handle, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd, ianaProtocolIPv6, syscall.IPV6_MULTICAST_LOOP, (*byte)(unsafe.Pointer(&vv)), 4))
}

func joinIPv6Group(fd syscall.Handle, ifi *net.Interface, grp net.IP) error {
	mreq := syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], grp)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd, ianaProtocolIPv6, syscall.IPV6_JOIN_GROUP, (*byte)(unsafe.Pointer(&mreq)), int32(unsafe.Sizeof(mreq))))
}

func leaveIPv6Group(fd syscall.Handle, ifi *net.Interface, grp net.IP) error {
	mreq := syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], grp)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd, ianaProtocolIPv6, syscall.IPV6_LEAVE_GROUP, (*byte)(unsafe.Pointer(&mreq)), int32(unsafe.Sizeof(mreq))))
}

func setIPv6Checksum(fd syscall.Handle, on bool, offset int) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}
