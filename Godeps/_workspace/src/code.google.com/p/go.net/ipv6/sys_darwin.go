// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"syscall"
)

// RFC 2292 options
const (
	// See /usr/include/netinet6/in6.h.
	sysSockopt2292HopLimit   = 0x14
	sysSockopt2292PacketInfo = 0x13
	sysSockopt2292NextHop    = 0x15
)

// RFC 3493 options
const (
	// See /usr/include/netinet6/in6.h.
	sysSockoptUnicastHopLimit    = 0x4
	sysSockoptMulticastHopLimit  = 0xa
	sysSockoptMulticastInterface = 0x9
	sysSockoptMulticastLoopback  = 0xb
	sysSockoptJoinGroup          = 0xc
	sysSockoptLeaveGroup         = 0xd
)

// RFC 3542 options
const (
	// See /usr/include/netinet6/in6.h.
	sysSockoptReceiveTrafficClass = 0x23
	sysSockoptTrafficClass        = 0x24
	sysSockoptReceiveHopLimit     = 0x25
	sysSockoptHopLimit            = 0x2f
	sysSockoptReceivePacketInfo   = 0x3d
	sysSockoptPacketInfo          = 0x2e
	sysSockoptReceivePathMTU      = 0x2b
	sysSockoptPathMTU             = 0x2c
	sysSockoptNextHop             = 0x30
	sysSockoptChecksum            = 0x1a

	// See /usr/include/netinet6/in6.h.
	sysSockoptICMPFilter = 0x12
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Len = syscall.SizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
