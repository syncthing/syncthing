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
func (c *Client) discover(conn net.PacketConn, addr *net.UDPAddr) (NATType, *Host, error) {
	// Perform test1 to check if it is under NAT.
	c.logger.Debugln("Do Test1")
	c.logger.Debugln("Send To:", addr)
	resp, err := c.test1(conn, addr)
	if err != nil {
		return NATError, nil, err
	}
	c.logger.Debugln("Received:", resp)
	if resp == nil {
		return NATBlocked, nil, nil
	}
	// identical used to check if it is open Internet or not.
	identical := resp.identical
	// changedAddr is used to perform second time test1 and test3.
	changedAddr := resp.changedAddr
	// mappedAddr is used as the return value, its IP is used for tests
	mappedAddr := resp.mappedAddr
	// Make sure IP and port are not changed.
	if resp.serverAddr.IP() != addr.IP.String() ||
		resp.serverAddr.Port() != uint16(addr.Port) {
		return NATError, mappedAddr, errors.New("Server error: response IP/port")
	}
	// if changedAddr is not available, use otherAddr as changedAddr,
	// which is updated in RFC 5780
	if changedAddr == nil {
		changedAddr = resp.otherAddr
	}
	// changedAddr shall not be nil
	if changedAddr == nil {
		return NATError, mappedAddr, errors.New("Server error: no changed address.")
	}
	// Perform test2 to see if the client can receive packet sent from
	// another IP and port.
	c.logger.Debugln("Do Test2")
	c.logger.Debugln("Send To:", addr)
	resp, err = c.test2(conn, addr)
	if err != nil {
		return NATError, mappedAddr, err
	}
	c.logger.Debugln("Received:", resp)
	// Make sure IP and port are changed.
	if resp != nil &&
		(resp.serverAddr.IP() == addr.IP.String() ||
			resp.serverAddr.Port() == uint16(addr.Port)) {
		return NATError, mappedAddr, errors.New("Server error: response IP/port")
	}
	if identical {
		if resp == nil {
			return NATSymetricUDPFirewall, mappedAddr, nil
		}
		return NATNone, mappedAddr, nil
	}
	if resp != nil {
		return NATFull, mappedAddr, nil
	}
	// Perform test1 to another IP and port to see if the NAT use the same
	// external IP.
	c.logger.Debugln("Do Test1")
	c.logger.Debugln("Send To:", changedAddr)
	caddr, err := net.ResolveUDPAddr("udp", changedAddr.String())
	resp, err = c.test1(conn, caddr)
	if err != nil {
		return NATError, mappedAddr, err
	}
	c.logger.Debugln("Received:", resp)
	if resp == nil {
		// It should be NAT_BLOCKED, but will be detected in the first
		// step. So this will never happen.
		return NATUnknown, mappedAddr, nil
	}
	// Make sure IP/port is not changed.
	if resp.serverAddr.IP() != caddr.IP.String() ||
		resp.serverAddr.Port() != uint16(caddr.Port) {
		return NATError, mappedAddr, errors.New("Server error: response IP/port")
	}
	if mappedAddr.IP() == resp.mappedAddr.IP() && mappedAddr.Port() == resp.mappedAddr.Port() {
		// Perform test3 to see if the client can receive packet sent
		// from another port.
		c.logger.Debugln("Do Test3")
		c.logger.Debugln("Send To:", caddr)
		resp, err = c.test3(conn, caddr)
		if err != nil {
			return NATError, mappedAddr, err
		}
		c.logger.Debugln("Received:", resp)
		if resp == nil {
			return NATPortRestricted, mappedAddr, nil
		}
		// Make sure IP is not changed, and port is changed.
		if resp.serverAddr.IP() != caddr.IP.String() ||
			resp.serverAddr.Port() == uint16(caddr.Port) {
			return NATError, mappedAddr, errors.New("Server error: response IP/port")
		}
		return NATRestricted, mappedAddr, nil
	}
	return NATSymetric, mappedAddr, nil
}
