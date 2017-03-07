// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux solaris

package ipv6

import (
	"net"
	"os"
	"unsafe"
)

var freebsd32o64 bool

func setsockoptGroupReq(s uintptr, opt *sockOpt, ifi *net.Interface, grp net.IP) error {
	var gr groupReq
	if ifi != nil {
		gr.Interface = uint32(ifi.Index)
	}
	gr.setGroup(grp)
	var p unsafe.Pointer
	var l uint32
	if freebsd32o64 {
		var d [sizeofGroupReq + 4]byte
		s := (*[sizeofGroupReq]byte)(unsafe.Pointer(&gr))
		copy(d[:4], s[:4])
		copy(d[8:], s[4:])
		p = unsafe.Pointer(&d[0])
		l = sizeofGroupReq + 4
	} else {
		p = unsafe.Pointer(&gr)
		l = sizeofGroupReq
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, opt.level, opt.name, p, l))
}

func setsockoptGroupSourceReq(s uintptr, opt *sockOpt, ifi *net.Interface, grp, src net.IP) error {
	var gsr groupSourceReq
	if ifi != nil {
		gsr.Interface = uint32(ifi.Index)
	}
	gsr.setSourceGroup(grp, src)
	var p unsafe.Pointer
	var l uint32
	if freebsd32o64 {
		var d [sizeofGroupSourceReq + 4]byte
		s := (*[sizeofGroupSourceReq]byte)(unsafe.Pointer(&gsr))
		copy(d[:4], s[:4])
		copy(d[8:], s[4:])
		p = unsafe.Pointer(&d[0])
		l = sizeofGroupSourceReq + 4
	} else {
		p = unsafe.Pointer(&gsr)
		l = sizeofGroupSourceReq
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, opt.level, opt.name, p, l))
}
