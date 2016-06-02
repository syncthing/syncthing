// Copyright 2013, Cong Ding. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: Cong Ding <dinggnu@gmail.com>

package stun

import (
	"errors"
	"net"
)

// Padding the length of the byte slice to multiple of 4.
func padding(bytes []byte) []byte {
	length := uint16(len(bytes))
	return append(bytes, make([]byte, align(length)-length)...)
}

// Align the uint16 number to the smallest multiple of 4, which is larger than
// or equal to the uint16 number.
func align(n uint16) uint16 {
	return (n + 3) & 0xfffc
}

func isLocalAddress(local, localRemote string) bool {
	// Resolve the IP returned by the STUN server first.
	localRemoteAddr, err := net.ResolveUDPAddr("udp", localRemote)
	if err != nil {
		return false
	}

	// Try comparing with the local address on the socket first, but only if
	// it's actually specified.
	addr, err := net.ResolveUDPAddr("udp", local)
	if err == nil && addr.IP != nil && !addr.IP.IsUnspecified() {
		return addr.IP.Equal(localRemoteAddr.IP)
	}

	// Fallback to checking IPs of all interfaces
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		if ip.Equal(localRemoteAddr.IP) {
			return true
		}
	}
	return false
}

func sendBindingReq(conn net.PacketConn, addr net.Addr, softwareName string) (*packet, error) {
	// Construct packet.
	packet := newPacket()
	packet.types = type_BINDING_REQUEST
	attribute := newSoftwareAttribute(packet, softwareName)
	packet.addAttribute(*attribute)
	attribute = newFingerprintAttribute(packet)
	packet.addAttribute(*attribute)
	// Send packet.
	packet, err := packet.send(conn, addr)
	if err != nil {
		return nil, err
	}
	return packet, nil
}

func sendChangeReq(conn net.PacketConn, addr net.Addr, softwareName string, changeIP bool, changePort bool) (*packet, error) {
	// Construct packet.
	packet := newPacket()
	packet.types = type_BINDING_REQUEST
	attribute := newSoftwareAttribute(packet, softwareName)
	packet.addAttribute(*attribute)
	attribute = newChangeReqAttribute(packet, changeIP, changePort)
	packet.addAttribute(*attribute)
	attribute = newFingerprintAttribute(packet)
	packet.addAttribute(*attribute)
	// Send packet.
	packet, err := packet.send(conn, addr)
	if err != nil {
		return nil, err
	}
	return packet, nil
}

func test1(conn net.PacketConn, addr net.Addr, softwareName string) (*packet, net.Addr, bool, *Host, error) {
	packet, err := sendBindingReq(conn, addr, softwareName)
	if err != nil {
		return nil, nil, false, nil, err
	}
	if packet == nil {
		return nil, nil, false, nil, nil
	}

	// RFC 3489 doesn't require the server return XOR mapped address.
	hostMappedAddr := packet.xorMappedAddr()
	if hostMappedAddr == nil {
		hostMappedAddr = packet.mappedAddr()
		if hostMappedAddr == nil {
			return nil, nil, false, nil, errors.New("No mapped address.")
		}
	}

	identical := isLocalAddress(conn.LocalAddr().String(), hostMappedAddr.TransportAddr())

	hostChangedAddr := packet.changedAddr()
	if hostChangedAddr == nil {
		return packet, nil, identical, hostMappedAddr, nil
	}
	changeAddrStr := hostChangedAddr.TransportAddr()
	changeAddr, err := net.ResolveUDPAddr("udp", changeAddrStr)
	if err != nil {
		return nil, nil, false, nil, errors.New("Failed to resolve changed address.")
	}
	return packet, changeAddr, identical, hostMappedAddr, nil
}

func test2(conn net.PacketConn, addr net.Addr, softwareName string) (*packet, error) {
	return sendChangeReq(conn, addr, softwareName, true, true)
}

func test3(conn net.PacketConn, addr net.Addr, softwareName string) (*packet, error) {
	return sendChangeReq(conn, addr, softwareName, false, true)
}

// Follow RFC 3489 and RFC 5389.
// Figure 2: Flow for type discovery process (from RFC 3489).
//                        +--------+
//                        |  Test  |
//                        |   I    |
//                        +--------+
//                             |
//                             |
//                             V
//                            /\              /\
//                         N /  \ Y          /  \ Y             +--------+
//          UDP     <-------/Resp\--------->/ IP \------------->|  Test  |
//          Blocked         \ ?  /          \Same/              |   II   |
//                           \  /            \? /               +--------+
//                            \/              \/                    |
//                                             | N                  |
//                                             |                    V
//                                             V                    /\
//                                         +--------+  Sym.      N /  \
//                                         |  Test  |  UDP    <---/Resp\
//                                         |   II   |  Firewall   \ ?  /
//                                         +--------+              \  /
//                                             |                    \/
//                                             V                     |Y
//                  /\                         /\                    |
//   Symmetric  N  /  \       +--------+   N  /  \                   V
//      NAT  <--- / IP \<-----|  Test  |<--- /Resp\               Open
//                \Same/      |   I    |     \ ?  /               Internet
//                 \? /       +--------+      \  /
//                  \/                         \/
//                  |Y                          |Y
//                  |                           |
//                  |                           V
//                  |                           Full
//                  |                           Cone
//                  V              /\
//              +--------+        /  \ Y
//              |  Test  |------>/Resp\---->Restricted
//              |   III  |       \ ?  /
//              +--------+        \  /
//                                 \/
//                                  |N
//                                  |       Port
//                                  +------>Restricted
func discover(conn net.PacketConn, addr net.Addr, softwareName string) (NATType, *Host, error) {
	packet, changeAddr, identical, host, err := test1(conn, addr, softwareName)
	if err != nil {
		return NAT_ERROR, nil, err
	}
	if packet == nil {
		return NAT_BLOCKED, nil, nil
	}
	packet, err = test2(conn, addr, softwareName)
	if err != nil {
		return NAT_ERROR, host, err
	}
	if identical {
		if packet == nil {
			return NAT_SYMETRIC_UDP_FIREWALL, host, nil
		}
		return NAT_NONE, host, nil
	}
	if packet != nil {
		return NAT_FULL, host, nil
	}

	otherConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return NAT_ERROR, nil, err
	}

	if changeAddr == nil {
		return NAT_ERROR, host, errors.New("No changed address.")
	}

	packet, _, identical, _, err = test1(otherConn, changeAddr, softwareName)
	if err != nil {
		return NAT_ERROR, host, err
	}
	if packet == nil {
		// It should be NAT_BLOCKED, but will be detected in the first
		// step. So this will never happen.
		return NAT_UNKNOWN, host, nil
	}
	if identical {
		packet, err = test3(conn, addr, softwareName)
		if err != nil {
			return NAT_ERROR, host, err
		}
		if packet == nil {
			return NAT_PORT_RESTRICTED, host, nil
		}
		return NAT_RESTRICTED, host, nil
	}
	return NAT_SYMETRIC, host, nil
}
