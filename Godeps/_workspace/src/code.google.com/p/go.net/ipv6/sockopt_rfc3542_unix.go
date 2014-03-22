// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build freebsd linux netbsd openbsd

package ipv6

import (
	"os"
	"unsafe"
)

func ipv6ReceiveTrafficClass(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptReceiveTrafficClass, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveTrafficClass(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptReceiveTrafficClass, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6ReceiveHopLimit(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptReceiveHopLimit, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveHopLimit(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptReceiveHopLimit, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6ReceivePacketInfo(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptReceivePacketInfo, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePacketInfo(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptReceivePacketInfo, uintptr(unsafe.Pointer(&vv)), 4))
}

func ipv6PathMTU(fd int) (int, error) {
	var v sysMTUInfo
	l := sysSockoptLen(sysSizeofMTUInfo)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptPathMTU, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v.MTU), nil
}

func ipv6ReceivePathMTU(fd int) (bool, error) {
	var v int32
	l := sysSockoptLen(4)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptReceivePathMTU, uintptr(unsafe.Pointer(&v)), &l); err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePathMTU(fd int, v bool) error {
	vv := int32(boolint(v))
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptReceivePathMTU, uintptr(unsafe.Pointer(&vv)), 4))
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
