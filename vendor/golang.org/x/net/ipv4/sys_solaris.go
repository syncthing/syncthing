// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"net"
	"syscall"
	"unsafe"
)

var (
	ctlOpts = [ctlMax]ctlOpt{
		ctlTTL:        {sysIP_RECVTTL, 4, marshalTTL, parseTTL},
		ctlPacketInfo: {sysIP_PKTINFO, sizeofInetPktinfo, marshalPacketInfo, parsePacketInfo},
	}

	sockOpts = [ssoMax]sockOpt{
		ssoTOS:                {sysIP_TOS, ssoTypeInt},
		ssoTTL:                {sysIP_TTL, ssoTypeInt},
		ssoMulticastTTL:       {sysIP_MULTICAST_TTL, ssoTypeByte},
		ssoMulticastInterface: {sysIP_MULTICAST_IF, ssoTypeInterface},
		ssoMulticastLoopback:  {sysIP_MULTICAST_LOOP, ssoTypeByte},
		ssoReceiveTTL:         {sysIP_RECVTTL, ssoTypeInt},
		ssoPacketInfo:         {sysIP_RECVPKTINFO, ssoTypeInt},
		ssoHeaderPrepend:      {sysIP_HDRINCL, ssoTypeInt},
		ssoJoinGroup:          {sysMCAST_JOIN_GROUP, ssoTypeGroupReq},
		ssoLeaveGroup:         {sysMCAST_LEAVE_GROUP, ssoTypeGroupReq},
		ssoJoinSourceGroup:    {sysMCAST_JOIN_SOURCE_GROUP, ssoTypeGroupSourceReq},
		ssoLeaveSourceGroup:   {sysMCAST_LEAVE_SOURCE_GROUP, ssoTypeGroupSourceReq},
		ssoBlockSourceGroup:   {sysMCAST_BLOCK_SOURCE, ssoTypeGroupSourceReq},
		ssoUnblockSourceGroup: {sysMCAST_UNBLOCK_SOURCE, ssoTypeGroupSourceReq},
	}
)

func (pi *inetPktinfo) setIfindex(i int) {
	pi.Ifindex = uint32(i)
}

func (gr *groupReq) setGroup(grp net.IP) {
	sa := (*sockaddrInet)(unsafe.Pointer(uintptr(unsafe.Pointer(gr)) + 4))
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], grp)
}

func (gsr *groupSourceReq) setSourceGroup(grp, src net.IP) {
	sa := (*sockaddrInet)(unsafe.Pointer(uintptr(unsafe.Pointer(gsr)) + 4))
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], grp)
	sa = (*sockaddrInet)(unsafe.Pointer(uintptr(unsafe.Pointer(gsr)) + 260))
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], src)
}
