// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/net/internal/iana"
)

var (
	ctlOpts = [ctlMax]ctlOpt{
		ctlHopLimit:   {sysIPV6_2292HOPLIMIT, 4, marshal2292HopLimit, parseHopLimit},
		ctlPacketInfo: {sysIPV6_2292PKTINFO, sizeofInet6Pktinfo, marshal2292PacketInfo, parsePacketInfo},
	}

	sockOpts = [ssoMax]sockOpt{
		ssoHopLimit:           {iana.ProtocolIPv6, sysIPV6_UNICAST_HOPS, ssoTypeInt},
		ssoMulticastInterface: {iana.ProtocolIPv6, sysIPV6_MULTICAST_IF, ssoTypeInterface},
		ssoMulticastHopLimit:  {iana.ProtocolIPv6, sysIPV6_MULTICAST_HOPS, ssoTypeInt},
		ssoMulticastLoopback:  {iana.ProtocolIPv6, sysIPV6_MULTICAST_LOOP, ssoTypeInt},
		ssoReceiveHopLimit:    {iana.ProtocolIPv6, sysIPV6_2292HOPLIMIT, ssoTypeInt},
		ssoReceivePacketInfo:  {iana.ProtocolIPv6, sysIPV6_2292PKTINFO, ssoTypeInt},
		ssoChecksum:           {iana.ProtocolIPv6, sysIPV6_CHECKSUM, ssoTypeInt},
		ssoICMPFilter:         {iana.ProtocolIPv6ICMP, sysICMP6_FILTER, ssoTypeICMPFilter},
		ssoJoinGroup:          {iana.ProtocolIPv6, sysIPV6_JOIN_GROUP, ssoTypeIPMreq},
		ssoLeaveGroup:         {iana.ProtocolIPv6, sysIPV6_LEAVE_GROUP, ssoTypeIPMreq},
	}
)

func init() {
	// Seems like kern.osreldate is veiled on latest OS X. We use
	// kern.osrelease instead.
	s, err := syscall.Sysctl("kern.osrelease")
	if err != nil {
		return
	}
	ss := strings.Split(s, ".")
	if len(ss) == 0 {
		return
	}
	// The IP_PKTINFO and protocol-independent multicast API were
	// introduced in OS X 10.7 (Darwin 11). But it looks like
	// those features require OS X 10.8 (Darwin 12) or above.
	// See http://support.apple.com/kb/HT1633.
	if mjver, err := strconv.Atoi(ss[0]); err != nil || mjver < 12 {
		return
	}
	ctlOpts[ctlTrafficClass] = ctlOpt{sysIPV6_TCLASS, 4, marshalTrafficClass, parseTrafficClass}
	ctlOpts[ctlHopLimit] = ctlOpt{sysIPV6_HOPLIMIT, 4, marshalHopLimit, parseHopLimit}
	ctlOpts[ctlPacketInfo] = ctlOpt{sysIPV6_PKTINFO, sizeofInet6Pktinfo, marshalPacketInfo, parsePacketInfo}
	ctlOpts[ctlNextHop] = ctlOpt{sysIPV6_NEXTHOP, sizeofSockaddrInet6, marshalNextHop, parseNextHop}
	ctlOpts[ctlPathMTU] = ctlOpt{sysIPV6_PATHMTU, sizeofIPv6Mtuinfo, marshalPathMTU, parsePathMTU}
	sockOpts[ssoTrafficClass] = sockOpt{iana.ProtocolIPv6, sysIPV6_TCLASS, ssoTypeInt}
	sockOpts[ssoReceiveTrafficClass] = sockOpt{iana.ProtocolIPv6, sysIPV6_RECVTCLASS, ssoTypeInt}
	sockOpts[ssoReceiveHopLimit] = sockOpt{iana.ProtocolIPv6, sysIPV6_RECVHOPLIMIT, ssoTypeInt}
	sockOpts[ssoReceivePacketInfo] = sockOpt{iana.ProtocolIPv6, sysIPV6_RECVPKTINFO, ssoTypeInt}
	sockOpts[ssoReceivePathMTU] = sockOpt{iana.ProtocolIPv6, sysIPV6_RECVPATHMTU, ssoTypeInt}
	sockOpts[ssoPathMTU] = sockOpt{iana.ProtocolIPv6, sysIPV6_PATHMTU, ssoTypeMTUInfo}
	sockOpts[ssoJoinGroup] = sockOpt{iana.ProtocolIPv6, sysMCAST_JOIN_GROUP, ssoTypeGroupReq}
	sockOpts[ssoLeaveGroup] = sockOpt{iana.ProtocolIPv6, sysMCAST_LEAVE_GROUP, ssoTypeGroupReq}
	sockOpts[ssoJoinSourceGroup] = sockOpt{iana.ProtocolIPv6, sysMCAST_JOIN_SOURCE_GROUP, ssoTypeGroupSourceReq}
	sockOpts[ssoLeaveSourceGroup] = sockOpt{iana.ProtocolIPv6, sysMCAST_LEAVE_SOURCE_GROUP, ssoTypeGroupSourceReq}
	sockOpts[ssoBlockSourceGroup] = sockOpt{iana.ProtocolIPv6, sysMCAST_BLOCK_SOURCE, ssoTypeGroupSourceReq}
	sockOpts[ssoUnblockSourceGroup] = sockOpt{iana.ProtocolIPv6, sysMCAST_UNBLOCK_SOURCE, ssoTypeGroupSourceReq}
}

func (sa *sockaddrInet6) setSockaddr(ip net.IP, i int) {
	sa.Len = sizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(i)
}

func (pi *inet6Pktinfo) setIfindex(i int) {
	pi.Ifindex = uint32(i)
}

func (mreq *ipv6Mreq) setIfindex(i int) {
	mreq.Interface = uint32(i)
}

func (gr *groupReq) setGroup(grp net.IP) {
	sa := (*sockaddrInet6)(unsafe.Pointer(uintptr(unsafe.Pointer(gr)) + 4))
	sa.Len = sizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], grp)
}

func (gsr *groupSourceReq) setSourceGroup(grp, src net.IP) {
	sa := (*sockaddrInet6)(unsafe.Pointer(uintptr(unsafe.Pointer(gsr)) + 4))
	sa.Len = sizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], grp)
	sa = (*sockaddrInet6)(unsafe.Pointer(uintptr(unsafe.Pointer(gsr)) + 132))
	sa.Len = sizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], src)
}
