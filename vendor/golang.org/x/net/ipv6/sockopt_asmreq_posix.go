// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd solaris windows

package ipv6

import (
	"net"
	"os"
	"unsafe"
)

func setsockoptIPMreq(s uintptr, opt *sockOpt, ifi *net.Interface, grp net.IP) error {
	var mreq ipv6Mreq
	copy(mreq.Multiaddr[:], grp)
	if ifi != nil {
		mreq.setIfindex(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, opt.level, opt.name, unsafe.Pointer(&mreq), sizeofIPv6Mreq))
}
