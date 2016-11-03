// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd windows

package ipv6

import (
	"net"
	"os"
	"unsafe"
)

func getInt(s uintptr, opt *sockOpt) (int, error) {
	if opt.name < 1 || opt.typ != ssoTypeInt {
		return 0, errOpNoSupport
	}
	var i int32
	l := uint32(4)
	if err := getsockopt(s, opt.level, opt.name, unsafe.Pointer(&i), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(i), nil
}

func setInt(s uintptr, opt *sockOpt, v int) error {
	if opt.name < 1 || opt.typ != ssoTypeInt {
		return errOpNoSupport
	}
	i := int32(v)
	return os.NewSyscallError("setsockopt", setsockopt(s, opt.level, opt.name, unsafe.Pointer(&i), 4))
}

func getInterface(s uintptr, opt *sockOpt) (*net.Interface, error) {
	if opt.name < 1 || opt.typ != ssoTypeInterface {
		return nil, errOpNoSupport
	}
	var i int32
	l := uint32(4)
	if err := getsockopt(s, opt.level, opt.name, unsafe.Pointer(&i), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	if i == 0 {
		return nil, nil
	}
	ifi, err := net.InterfaceByIndex(int(i))
	if err != nil {
		return nil, err
	}
	return ifi, nil
}

func setInterface(s uintptr, opt *sockOpt, ifi *net.Interface) error {
	if opt.name < 1 || opt.typ != ssoTypeInterface {
		return errOpNoSupport
	}
	var i int32
	if ifi != nil {
		i = int32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, opt.level, opt.name, unsafe.Pointer(&i), 4))
}

func getICMPFilter(s uintptr, opt *sockOpt) (*ICMPFilter, error) {
	if opt.name < 1 || opt.typ != ssoTypeICMPFilter {
		return nil, errOpNoSupport
	}
	var f ICMPFilter
	l := uint32(sysSizeofICMPv6Filter)
	if err := getsockopt(s, opt.level, opt.name, unsafe.Pointer(&f.sysICMPv6Filter), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	return &f, nil
}

func setICMPFilter(s uintptr, opt *sockOpt, f *ICMPFilter) error {
	if opt.name < 1 || opt.typ != ssoTypeICMPFilter {
		return errOpNoSupport
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, opt.level, opt.name, unsafe.Pointer(&f.sysICMPv6Filter), sysSizeofICMPv6Filter))
}

func getMTUInfo(s uintptr, opt *sockOpt) (*net.Interface, int, error) {
	if opt.name < 1 || opt.typ != ssoTypeMTUInfo {
		return nil, 0, errOpNoSupport
	}
	var mi sysIPv6Mtuinfo
	l := uint32(sysSizeofIPv6Mtuinfo)
	if err := getsockopt(s, opt.level, opt.name, unsafe.Pointer(&mi), &l); err != nil {
		return nil, 0, os.NewSyscallError("getsockopt", err)
	}
	if mi.Addr.Scope_id == 0 {
		return nil, int(mi.Mtu), nil
	}
	ifi, err := net.InterfaceByIndex(int(mi.Addr.Scope_id))
	if err != nil {
		return nil, 0, err
	}
	return ifi, int(mi.Mtu), nil
}

func setGroup(s uintptr, opt *sockOpt, ifi *net.Interface, grp net.IP) error {
	if opt.name < 1 {
		return errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeIPMreq:
		return setsockoptIPMreq(s, opt, ifi, grp)
	case ssoTypeGroupReq:
		return setsockoptGroupReq(s, opt, ifi, grp)
	default:
		return errOpNoSupport
	}
}

func setSourceGroup(s uintptr, opt *sockOpt, ifi *net.Interface, grp, src net.IP) error {
	if opt.name < 1 || opt.typ != ssoTypeGroupSourceReq {
		return errOpNoSupport
	}
	return setsockoptGroupSourceReq(s, opt, ifi, grp, src)
}
