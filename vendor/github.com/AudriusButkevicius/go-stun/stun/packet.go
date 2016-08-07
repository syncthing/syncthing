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
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"
)

var debug = false

// SetDebug sets the library to debug mode, which prints all the network traffic.
func SetDebug(d bool) {
	debug = d
}

type packet struct {
	types      uint16
	length     uint16
	id         []byte // 4 bytes magic cookie + 12 bytes transaction id
	attributes []attribute
}

func newPacket() (*packet, error) {
	v := new(packet)
	v.id = make([]byte, 16)
	binary.BigEndian.PutUint32(v.id[:4], magicCookie)
	_, err := rand.Read(v.id[4:])
	if err != nil {
		return nil, err
	}
	v.attributes = make([]attribute, 0, 10)
	v.length = 0
	return v, nil
}

func newPacketFromBytes(packetBytes []byte) (*packet, error) {
	if len(packetBytes) < 24 {
		return nil, errors.New("Received data length too short.")
	}
	packet, err := newPacket()
	if err != nil {
		return nil, err
	}
	packet.types = binary.BigEndian.Uint16(packetBytes[0:2])
	packet.length = binary.BigEndian.Uint16(packetBytes[2:4])
	packet.id = packetBytes[4:20]
	for pos := uint16(20); pos < uint16(len(packetBytes)); {
		types := binary.BigEndian.Uint16(packetBytes[pos : pos+2])
		length := binary.BigEndian.Uint16(packetBytes[pos+2 : pos+4])
		if pos+4+length > uint16(len(packetBytes)) {
			return nil, errors.New("Received data format mismatch.")
		}
		value := packetBytes[pos+4 : pos+4+length]
		attribute := newAttribute(types, value)
		packet.addAttribute(*attribute)
		pos += align(length) + 4
	}
	return packet, nil
}

func (v *packet) addAttribute(a attribute) {
	v.attributes = append(v.attributes, a)
	v.length += align(a.length) + 4
}

func (v *packet) bytes() []byte {
	packetBytes := make([]byte, 4)
	binary.BigEndian.PutUint16(packetBytes[0:2], v.types)
	binary.BigEndian.PutUint16(packetBytes[2:4], v.length)
	packetBytes = append(packetBytes, v.id...)
	for _, a := range v.attributes {
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, a.types)
		packetBytes = append(packetBytes, buf...)
		binary.BigEndian.PutUint16(buf, a.length)
		packetBytes = append(packetBytes, buf...)
		packetBytes = append(packetBytes, a.value...)
	}
	return packetBytes
}

func (v *packet) sourceAddr() *Host {
	for _, a := range v.attributes {
		if a.types == attribute_SOURCE_ADDRESS {
			return a.address()
		}
	}
	return nil
}

func (v *packet) mappedAddr() *Host {
	for _, a := range v.attributes {
		if a.types == attribute_MAPPED_ADDRESS {
			return a.address()
		}
	}
	return nil
}

func (v *packet) changeAddr() *Host {
	for _, a := range v.attributes {
		if a.types == attribute_CHANGED_ADDRESS {
			return a.address()
		}
	}
	return nil
}

func (v *packet) xorMappedAddr() *Host {
	for _, a := range v.attributes {
		if (a.types == attribute_XOR_MAPPED_ADDRESS) || (a.types == attribute_XOR_MAPPED_ADDRESS_EXP) {
			return a.xorMappedAddr(v.id)
		}
	}
	return nil
}

// RFC 3489: Clients SHOULD retransmit the request starting with an interval
// of 100ms, doubling every retransmit until the interval reaches 1.6s.
// Retransmissions continue with intervals of 1.6s until a response is
// received, or a total of 9 requests have been sent.
func (v *packet) send(conn net.PacketConn, addr net.Addr) (net.Addr, *packet, error) {
	if debug {
		fmt.Print(hex.Dump(v.bytes()))
	}
	timeout := 100
	for i := 0; i < 9; i++ {
		length, err := conn.WriteTo(v.bytes(), addr)
		if err != nil {
			return nil, nil, err
		}
		if length != len(v.bytes()) {
			return nil, nil, errors.New("Error in sending data.")
		}
		err = conn.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond))
		if err != nil {
			return nil, nil, err
		}
		if timeout < 1600 {
			timeout *= 2
		}
		packetBytes := make([]byte, 1024)
		length, raddr, err := conn.ReadFrom(packetBytes)
		if err == nil {
			pkt, err := newPacketFromBytes(packetBytes[0:length])
			if debug {
				fmt.Print(hex.Dump(pkt.bytes()))
			}
			return raddr, pkt, err
		}
		if !err.(net.Error).Timeout() {
			return nil, nil, err
		}
	}
	return nil, nil, nil
}
