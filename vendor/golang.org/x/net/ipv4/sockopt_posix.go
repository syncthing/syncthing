// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd solaris windows

package ipv4

import (
	"net"
	"os"
	"unsafe"

	"golang.org/x/net/internal/iana"
)

func getInt(s uintptr, opt *sockOpt) (int, error) {
	if opt.name < 1 || (opt.typ != ssoTypeByte && opt.typ != ssoTypeInt) {
		return 0, errOpNoSupport
	}
	var i int32
	var b byte
	p := unsafe.Pointer(&i)
	l := uint32(4)
	if opt.typ == ssoTypeByte {
		p = unsafe.Pointer(&b)
		l = 1
	}
	if err := getsockopt(s, iana.ProtocolIP, opt.name, p, &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	if opt.typ == ssoTypeByte {
		return int(b), nil
	}
	return int(i), nil
}

func setInt(s uintptr, opt *sockOpt, v int) error {
	if opt.name < 1 || (opt.typ != ssoTypeByte && opt.typ != ssoTypeInt) {
		return errOpNoSupport
	}
	i := int32(v)
	var b byte
	p := unsafe.Pointer(&i)
	l := uint32(4)
	if opt.typ == ssoTypeByte {
		b = byte(v)
		p = unsafe.Pointer(&b)
		l = 1
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, iana.ProtocolIP, opt.name, p, l))
}

func getInterface(s uintptr, opt *sockOpt) (*net.Interface, error) {
	if opt.name < 1 {
		return nil, errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeInterface:
		return getsockoptInterface(s, opt.name)
	case ssoTypeIPMreqn:
		return getsockoptIPMreqn(s, opt.name)
	default:
		return nil, errOpNoSupport
	}
}

func setInterface(s uintptr, opt *sockOpt, ifi *net.Interface) error {
	if opt.name < 1 {
		return errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeInterface:
		return setsockoptInterface(s, opt.name, ifi)
	case ssoTypeIPMreqn:
		return setsockoptIPMreqn(s, opt.name, ifi, nil)
	default:
		return errOpNoSupport
	}
}

func getICMPFilter(s uintptr, opt *sockOpt) (*ICMPFilter, error) {
	if opt.name < 1 || opt.typ != ssoTypeICMPFilter {
		return nil, errOpNoSupport
	}
	var f ICMPFilter
	l := uint32(sizeofICMPFilter)
	if err := getsockopt(s, iana.ProtocolReserved, opt.name, unsafe.Pointer(&f.icmpFilter), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	return &f, nil
}

func setICMPFilter(s uintptr, opt *sockOpt, f *ICMPFilter) error {
	if opt.name < 1 || opt.typ != ssoTypeICMPFilter {
		return errOpNoSupport
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, iana.ProtocolReserved, opt.name, unsafe.Pointer(&f.icmpFilter), sizeofICMPFilter))
}

func setGroup(s uintptr, opt *sockOpt, ifi *net.Interface, grp net.IP) error {
	if opt.name < 1 {
		return errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeIPMreq:
		return setsockoptIPMreq(s, opt.name, ifi, grp)
	case ssoTypeIPMreqn:
		return setsockoptIPMreqn(s, opt.name, ifi, grp)
	case ssoTypeGroupReq:
		return setsockoptGroupReq(s, opt.name, ifi, grp)
	default:
		return errOpNoSupport
	}
}

func setSourceGroup(s uintptr, opt *sockOpt, ifi *net.Interface, grp, src net.IP) error {
	if opt.name < 1 || opt.typ != ssoTypeGroupSourceReq {
		return errOpNoSupport
	}
	return setsockoptGroupSourceReq(s, opt.name, ifi, grp, src)
}
