// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin

package ipv6

import (
	"os"
	"unsafe"
)

func ipv6ReceiveTrafficClass(fd int) (bool, error) {
	return false, errOpNoSupport
}

func setIPv6ReceiveTrafficClass(fd int, v bool) error {
	return errOpNoSupport
}

func ipv6ReceiveHopLimit(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockopt2292HopLimit, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveHopLimit(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockopt2292HopLimit, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6ReceivePacketInfo(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockopt2292PacketInfo, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePacketInfo(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockopt2292PacketInfo, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6PathMTU(fd int) (int, error) {
	return 0, errOpNoSupport
}

func ipv6ReceivePathMTU(fd int) (bool, error) {
	return false, errOpNoSupport
}

func setIPv6ReceivePathMTU(fd int, v bool) error {
	return errOpNoSupport
}

func ipv6ICMPFilter(fd int) (*ICMPFilter, error) {
	var v ICMPFilter
	l := sysSockoptLen(sysSizeofICMPFilter)
	if err := getsockopt(fd, ianaProtocolIPv6ICMP, sysSockoptICMPFilter, uintptr(unsafe.Pointer(&v.sysICMPFilter)), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	return &v, nil
}

func setIPv6ICMPFilter(fd int, f *ICMPFilter) error {
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6ICMP, sysSockoptICMPFilter, uintptr(unsafe.Pointer(&f.sysICMPFilter)), sysSizeofICMPFilter))
}
