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
	"errors"
	"net"
	"time"
)

type packet struct {
	types      uint16
	length     uint16
	cookie     uint32
	id         []byte // 12 bytes
	attributes []attribute
}

func newPacket() *packet {
	v := new(packet)
	v.id = make([]byte, 12)
	rand.Read(v.id)
	v.attributes = make([]attribute, 0, 10)
	v.cookie = magicCookie
	v.length = 0
	return v
}

func newPacketFromBytes(packetBytes []byte) (*packet, error) {
	if len(packetBytes) < 24 {
		return nil, errors.New("Received data length too short.")
	}
	packet := newPacket()
	packet.types = binary.BigEndian.Uint16(packetBytes[0:2])
	packet.length = binary.BigEndian.Uint16(packetBytes[2:4])
	packet.cookie = binary.BigEndian.Uint32(packetBytes[4:8])
	packet.id = packetBytes[8:20]
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
	packetBytes := make([]byte, 8)
	binary.BigEndian.PutUint16(packetBytes[0:2], v.types)
	binary.BigEndian.PutUint16(packetBytes[2:4], v.length)
	binary.BigEndian.PutUint32(packetBytes[4:8], v.cookie)
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

func (v *packet) mappedAddr() *Host {
	for _, a := range v.attributes {
		if a.types == attribute_MAPPED_ADDRESS {
			return a.address()
		}
	}
	return nil
}

func (v *packet) changedAddr() *Host {
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
			return a.xorMappedAddr()
		}
	}
	return nil
}

// RFC 3489: Clients SHOULD retransmit the request starting with an interval
// of 100ms, doubling every retransmit until the interval reaches 1.6s.
// Retransmissions continue with intervals of 1.6s until a response is
// received, or a total of 9 requests have been sent.
func (v *packet) send(conn net.PacketConn, addr net.Addr) (*packet, error) {
	timeout := 100
	for i := 0; i < 9; i++ {
		length, err := conn.WriteTo(v.bytes(), addr)
		if err != nil {
			return nil, err
		}
		if length != len(v.bytes()) {
			return nil, errors.New("Error in sending data.")
		}
		conn.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond))
		if timeout < 1600 {
			timeout *= 2
		}
		packetBytes := make([]byte, 1024)
		length, _, err = conn.ReadFrom(packetBytes)
		if err == nil {
			return newPacketFromBytes(packetBytes[0:length])
		} else {
			if !err.(net.Error).Timeout() {
				return nil, err
			}
		}
	}
	return nil, nil
}
