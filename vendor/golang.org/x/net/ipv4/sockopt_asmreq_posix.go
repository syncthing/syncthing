// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd netbsd openbsd solaris windows

package ipv4

import (
	"net"
	"os"
	"unsafe"

	"golang.org/x/net/internal/iana"
)

func setsockoptIPMreq(s uintptr, name int, ifi *net.Interface, grp net.IP) error {
	mreq := ipMreq{Multiaddr: [4]byte{grp[0], grp[1], grp[2], grp[3]}}
	if err := setIPMreqInterface(&mreq, ifi); err != nil {
		return err
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, iana.ProtocolIP, name, unsafe.Pointer(&mreq), sizeofIPMreq))
}

func getsockoptInterface(s uintptr, name int) (*net.Interface, error) {
	var b [4]byte
	l := uint32(4)
	if err := getsockopt(s, iana.ProtocolIP, name, unsafe.Pointer(&b[0]), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	ifi, err := netIP4ToInterface(net.IPv4(b[0], b[1], b[2], b[3]))
	if err != nil {
		return nil, err
	}
	return ifi, nil
}

func setsockoptInterface(s uintptr, name int, ifi *net.Interface) error {
	ip, err := netInterfaceToIP4(ifi)
	if err != nil {
		return err
	}
	var b [4]byte
	copy(b[:], ip)
	return os.NewSyscallError("setsockopt", setsockopt(s, iana.ProtocolIP, name, unsafe.Pointer(&b[0]), uint32(4)))
}
