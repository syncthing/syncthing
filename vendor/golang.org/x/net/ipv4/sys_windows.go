// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

const (
	// See ws2tcpip.h.
	sysIP_OPTIONS                = 0x1
	sysIP_HDRINCL                = 0x2
	sysIP_TOS                    = 0x3
	sysIP_TTL                    = 0x4
	sysIP_MULTICAST_IF           = 0x9
	sysIP_MULTICAST_TTL          = 0xa
	sysIP_MULTICAST_LOOP         = 0xb
	sysIP_ADD_MEMBERSHIP         = 0xc
	sysIP_DROP_MEMBERSHIP        = 0xd
	sysIP_DONTFRAGMENT           = 0xe
	sysIP_ADD_SOURCE_MEMBERSHIP  = 0xf
	sysIP_DROP_SOURCE_MEMBERSHIP = 0x10
	sysIP_PKTINFO                = 0x13

	sizeofInetPktinfo  = 0x8
	sizeofIPMreq       = 0x8
	sizeofIPMreqSource = 0xc
)

type inetPktinfo struct {
	Addr    [4]byte
	Ifindex int32
}

type ipMreq struct {
	Multiaddr [4]byte
	Interface [4]byte
}

type ipMreqSource struct {
	Multiaddr  [4]byte
	Sourceaddr [4]byte
	Interface  [4]byte
}

// See http://msdn.microsoft.com/en-us/library/windows/desktop/ms738586(v=vs.85).aspx
var (
	ctlOpts = [ctlMax]ctlOpt{}

	sockOpts = [ssoMax]sockOpt{
		ssoTOS:                {sysIP_TOS, ssoTypeInt},
		ssoTTL:                {sysIP_TTL, ssoTypeInt},
		ssoMulticastTTL:       {sysIP_MULTICAST_TTL, ssoTypeInt},
		ssoMulticastInterface: {sysIP_MULTICAST_IF, ssoTypeInterface},
		ssoMulticastLoopback:  {sysIP_MULTICAST_LOOP, ssoTypeInt},
		ssoHeaderPrepend:      {sysIP_HDRINCL, ssoTypeInt},
		ssoJoinGroup:          {sysIP_ADD_MEMBERSHIP, ssoTypeIPMreq},
		ssoLeaveGroup:         {sysIP_DROP_MEMBERSHIP, ssoTypeIPMreq},
	}
)

func (pi *inetPktinfo) setIfindex(i int) {
	pi.Ifindex = int32(i)
}
