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
	// See /usr/include/linux/in6.h.
	sysSockoptUnicastHopLimit    = 0x10
	sysSockoptMulticastHopLimit  = 0x12
	sysSockoptMulticastInterface = 0x11
	sysSockoptMulticastLoopback  = 0x13
	sysSockoptJoinGroup          = 0x14
	sysSockoptLeaveGroup         = 0x15
)

// RFC 3542 options
const (
	// See /usr/include/linux/ipv6.h,in6.h.
	sysSockoptReceiveTrafficClass = 0x42
	sysSockoptTrafficClass        = 0x43
	sysSockoptReceiveHopLimit     = 0x33
	sysSockoptHopLimit            = 0x34
	sysSockoptReceivePacketInfo   = 0x31
	sysSockoptPacketInfo          = 0x32
	sysSockoptReceivePathMTU      = 0x3c
	sysSockoptPathMTU             = 0x3d
	sysSockoptNextHop             = 0x9
	sysSockoptChecksum            = 0x7

	// See /usr/include/linux/icmpv6.h.
	sysSockoptICMPFilter = 0x1
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
