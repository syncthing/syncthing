// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd

package ipv6

import (
	"net"
	"os"
	"unsafe"
)

func ipv6TrafficClass(fd int) (int, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptTrafficClass, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v), nil
}

func setIPv6TrafficClass(fd, v int) error {
	vv := int32(v)
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptTrafficClass, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6HopLimit(fd int) (int, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptUnicastHopLimit, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v), nil
}

func setIPv6HopLimit(fd, v int) error {
	vv := int32(v)
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptUnicastHopLimit, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6Checksum(fd int) (bool, int, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptChecksum, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, 0, os.NewSyscallError("getsockopt", err)
	}
	on := true
	if v == -1 {
		on = false
	}
	return on, int(v), nil
}

func ipv6MulticastHopLimit(fd int) (int, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptMulticastHopLimit, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v), nil
}

func setIPv6MulticastHopLimit(fd, v int) error {
	vv := int32(v)
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptMulticastHopLimit, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6MulticastInterface(fd int) (*net.Interface, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptMulticastInterface, uintptr(unsafe.Pointer(&v)), &l); err != nil {
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

func setIPv6MulticastInterface(fd int, ifi *net.Interface) error {
	var v int32
	if ifi != nil {
		v = int32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptMulticastInterface, uintptr(unsafe.Pointer(&v)), 4))
}

func ipv6MulticastLoopback(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptMulticastLoopback, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6MulticastLoopback(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptMulticastLoopback, uintptr(unsafe.Pointer(&vv)), 4))
}

func joinIPv6Group(fd int, ifi *net.Interface, grp net.IP) error {
	mreq := sysMulticastReq{}
	copy(mreq.IP[:], grp)
	if ifi != nil {
		mreq.IfIndex = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptJoinGroup, uintptr(unsafe.Pointer(&mreq)), sysSizeofMulticastReq))
}

func leaveIPv6Group(fd int, ifi *net.Interface, grp net.IP) error {
	mreq := sysMulticastReq{}
	copy(mreq.IP[:], grp)
	if ifi != nil {
		mreq.IfIndex = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptLeaveGroup, uintptr(unsafe.Pointer(&mreq)), sysSizeofMulticastReq))
}
