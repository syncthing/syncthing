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
)

type packet struct {
	types      uint16
	length     uint16
	transID    []byte // 4 bytes magic cookie + 12 bytes transaction id
	attributes []attribute
}

func newPacket() (*packet, error) {
	v := new(packet)
	v.transID = make([]byte, 16)
	binary.BigEndian.PutUint32(v.transID[:4], magicCookie)
	_, err := rand.Read(v.transID[4:])
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
	pkt := new(packet)
	pkt.types = binary.BigEndian.Uint16(packetBytes[0:2])
	pkt.length = binary.BigEndian.Uint16(packetBytes[2:4])
	pkt.transID = packetBytes[4:20]
	pkt.attributes = make([]attribute, 0, 10)
	for pos := uint16(20); pos < uint16(len(packetBytes)); {
		types := binary.BigEndian.Uint16(packetBytes[pos : pos+2])
		length := binary.BigEndian.Uint16(packetBytes[pos+2 : pos+4])
		if pos+4+length > uint16(len(packetBytes)) {
			return nil, errors.New("Received data format mismatch.")
		}
		value := packetBytes[pos+4 : pos+4+length]
		attribute := newAttribute(types, value)
		pkt.addAttribute(*attribute)
		pos += align(length) + 4
	}
	return pkt, nil
}

func (v *packet) addAttribute(a attribute) {
	v.attributes = append(v.attributes, a)
	v.length += align(a.length) + 4
}

func (v *packet) bytes() []byte {
	packetBytes := make([]byte, 4)
	binary.BigEndian.PutUint16(packetBytes[0:2], v.types)
	binary.BigEndian.PutUint16(packetBytes[2:4], v.length)
	packetBytes = append(packetBytes, v.transID...)
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

func (v *packet) getSourceAddr() *Host {
	return v.getRawAddr(attributeSourceAddress)
}

func (v *packet) getMappedAddr() *Host {
	return v.getRawAddr(attributeMappedAddress)
}

func (v *packet) getChangedAddr() *Host {
	return v.getRawAddr(attributeChangedAddress)
}

func (v *packet) getOtherAddr() *Host {
	return v.getRawAddr(attributeOtherAddress)
}

func (v *packet) getRawAddr(attribute uint16) *Host {
	for _, a := range v.attributes {
		if a.types == attribute {
			return a.rawAddr()
		}
	}
	return nil
}

func (v *packet) getXorMappedAddr() *Host {
	addr := v.getXorAddr(attributeXorMappedAddress)
	if addr == nil {
		addr = v.getXorAddr(attributeXorMappedAddressExp)
	}
	return addr
}

func (v *packet) getXorAddr(attribute uint16) *Host {
	for _, a := range v.attributes {
		if a.types == attribute {
			return a.xorAddr(v.transID)
		}
	}
	return nil
}
