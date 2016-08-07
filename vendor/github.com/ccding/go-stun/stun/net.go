// Copyright 2016, Cong Ding. All rights reserved.
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
	"bytes"
	"encoding/hex"
	"errors"
	"net"
	"time"
)

const (
	numRetransmit  = 9
	defaultTimeout = 100
	maxTimeout     = 1600
	maxPacketSize  = 1024
)

func (c *Client) sendBindingReq(conn net.PacketConn, addr net.Addr, changeIP bool, changePort bool) (*response, error) {
	// Construct packet.
	pkt, err := newPacket()
	if err != nil {
		return nil, err
	}
	pkt.types = typeBindingRequest
	attribute := newSoftwareAttribute(c.softwareName)
	pkt.addAttribute(*attribute)
	if changeIP || changePort {
		attribute = newChangeReqAttribute(changeIP, changePort)
		pkt.addAttribute(*attribute)
	}
	attribute = newFingerprintAttribute(pkt)
	pkt.addAttribute(*attribute)
	// Send packet.
	return c.send(pkt, conn, addr)
}

// RFC 3489: Clients SHOULD retransmit the request starting with an interval
// of 100ms, doubling every retransmit until the interval reaches 1.6s.
// Retransmissions continue with intervals of 1.6s until a response is
// received, or a total of 9 requests have been sent.
func (c *Client) send(pkt *packet, conn net.PacketConn, addr net.Addr) (*response, error) {
	c.logger.Info("\n" + hex.Dump(pkt.bytes()))
	timeout := defaultTimeout
	packetBytes := make([]byte, maxPacketSize)
	for i := 0; i < numRetransmit; i++ {
		// Send packet to the server.
		length, err := conn.WriteTo(pkt.bytes(), addr)
		if err != nil {
			return nil, err
		}
		if length != len(pkt.bytes()) {
			return nil, errors.New("Error in sending data.")
		}
		err = conn.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond))
		if err != nil {
			return nil, err
		}
		if timeout < maxTimeout {
			timeout *= 2
		}
		for {
			// Read from the port.
			length, raddr, err := conn.ReadFrom(packetBytes)
			if err != nil {
				if err.(net.Error).Timeout() {
					break
				}
				return nil, err
			}
			p, err := newPacketFromBytes(packetBytes[0:length])
			if err != nil {
				return nil, err
			}
			// If transId mismatches, keep reading until get a
			// matched packet or timeout.
			if !bytes.Equal(pkt.transID, p.transID) {
				continue
			}
			c.logger.Info("\n" + hex.Dump(packetBytes[0:length]))
			resp := newResponse(p, conn)
			resp.serverAddr = newHostFromStr(raddr.String())
			return resp, err
		}
	}
	return nil, nil
}
