// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"net"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	ctlOpts = [ctlMax]ctlOpt{
		ctlTTL:       {sysIP_RECVTTL, 1, marshalTTL, parseTTL},
		ctlDst:       {sysIP_RECVDSTADDR, net.IPv4len, marshalDst, parseDst},
		ctlInterface: {sysIP_RECVIF, syscall.SizeofSockaddrDatalink, marshalInterface, parseInterface},
	}

	sockOpts = [ssoMax]sockOpt{
		ssoTOS:                {sysIP_TOS, ssoTypeInt},
		ssoTTL:                {sysIP_TTL, ssoTypeInt},
		ssoMulticastTTL:       {sysIP_MULTICAST_TTL, ssoTypeByte},
		ssoMulticastInterface: {sysIP_MULTICAST_IF, ssoTypeInterface},
		ssoMulticastLoopback:  {sysIP_MULTICAST_LOOP, ssoTypeInt},
		ssoReceiveTTL:         {sysIP_RECVTTL, ssoTypeInt},
		ssoReceiveDst:         {sysIP_RECVDSTADDR, ssoTypeInt},
		ssoReceiveInterface:   {sysIP_RECVIF, ssoTypeInt},
		ssoHeaderPrepend:      {sysIP_HDRINCL, ssoTypeInt},
		ssoStripHeader:        {sysIP_STRIPHDR, ssoTypeInt},
		ssoJoinGroup:          {sysIP_ADD_MEMBERSHIP, ssoTypeIPMreq},
		ssoLeaveGroup:         {sysIP_DROP_MEMBERSHIP, ssoTypeIPMreq},
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
	ctlOpts[ctlPacketInfo].name = sysIP_PKTINFO
	ctlOpts[ctlPacketInfo].length = sizeofInetPktinfo
	ctlOpts[ctlPacketInfo].marshal = marshalPacketInfo
	ctlOpts[ctlPacketInfo].parse = parsePacketInfo
	sockOpts[ssoPacketInfo].name = sysIP_RECVPKTINFO
	sockOpts[ssoPacketInfo].typ = ssoTypeInt
	sockOpts[ssoMulticastInterface].typ = ssoTypeIPMreqn
	sockOpts[ssoJoinGroup].name = sysMCAST_JOIN_GROUP
	sockOpts[ssoJoinGroup].typ = ssoTypeGroupReq
	sockOpts[ssoLeaveGroup].name = sysMCAST_LEAVE_GROUP
	sockOpts[ssoLeaveGroup].typ = ssoTypeGroupReq
	sockOpts[ssoJoinSourceGroup].name = sysMCAST_JOIN_SOURCE_GROUP
	sockOpts[ssoJoinSourceGroup].typ = ssoTypeGroupSourceReq
	sockOpts[ssoLeaveSourceGroup].name = sysMCAST_LEAVE_SOURCE_GROUP
	sockOpts[ssoLeaveSourceGroup].typ = ssoTypeGroupSourceReq
	sockOpts[ssoBlockSourceGroup].name = sysMCAST_BLOCK_SOURCE
	sockOpts[ssoBlockSourceGroup].typ = ssoTypeGroupSourceReq
	sockOpts[ssoUnblockSourceGroup].name = sysMCAST_UNBLOCK_SOURCE
	sockOpts[ssoUnblockSourceGroup].typ = ssoTypeGroupSourceReq
}

func (pi *inetPktinfo) setIfindex(i int) {
	pi.Ifindex = uint32(i)
}

func (gr *groupReq) setGroup(grp net.IP) {
	sa := (*sockaddrInet)(unsafe.Pointer(uintptr(unsafe.Pointer(gr)) + 4))
	sa.Len = sizeofSockaddrInet
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], grp)
}

func (gsr *groupSourceReq) setSourceGroup(grp, src net.IP) {
	sa := (*sockaddrInet)(unsafe.Pointer(uintptr(unsafe.Pointer(gsr)) + 4))
	sa.Len = sizeofSockaddrInet
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], grp)
	sa = (*sockaddrInet)(unsafe.Pointer(uintptr(unsafe.Pointer(gsr)) + 132))
	sa.Len = sizeofSockaddrInet
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], src)
}
