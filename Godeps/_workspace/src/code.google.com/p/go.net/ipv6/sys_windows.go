// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"syscall"
)

// RFC 3493 options
const (
	// See ws2tcpip.h.
	sysSockoptUnicastHopLimit    = 0x4
	sysSockoptMulticastHopLimit  = 0xa
	sysSockoptMulticastInterface = 0x9
	sysSockoptMulticastLoopback  = 0xb
	sysSockoptJoinGroup          = 0xc
	sysSockoptLeaveGroup         = 0xd
)

// RFC 3542 options
const (
	// See ws2tcpip.h.
	sysSockoptPacketInfo = 0x13
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
