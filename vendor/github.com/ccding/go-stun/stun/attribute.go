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
	"encoding/binary"
	"hash/crc32"
	"net"
)

type attribute struct {
	types  uint16
	length uint16
	value  []byte
}

func newAttribute(types uint16, value []byte) *attribute {
	att := new(attribute)
	att.types = types
	att.value = padding(value)
	att.length = uint16(len(att.value))
	return att
}

func newFingerprintAttribute(packet *packet) *attribute {
	crc := crc32.ChecksumIEEE(packet.bytes()) ^ fingerprint
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, crc)
	return newAttribute(attribute_FINGERPRINT, buf)
}

func newSoftwareAttribute(packet *packet, name string) *attribute {
	return newAttribute(attribute_SOFTWARE, []byte(name))
}

func newChangeReqAttribute(packet *packet, changeIP bool, changePort bool) *attribute {
	value := make([]byte, 4)
	if changeIP {
		value[3] |= 0x04
	}
	if changePort {
		value[3] |= 0x02
	}
	return newAttribute(attribute_CHANGE_REQUEST, value)
}

func (v *attribute) xorMappedAddr() *Host {
	cookie := make([]byte, 4)
	binary.BigEndian.PutUint32(cookie, magicCookie)
	xorIP := make([]byte, 16)
	for i := 0; i < len(v.value)-4; i++ {
		xorIP[i] = v.value[i+4] ^ cookie[i]
	}
	family := binary.BigEndian.Uint16(v.value[0:2])
	port := binary.BigEndian.Uint16(v.value[2:4])

	// Truncate if IPv4, otherwise net.IP sometimes renders it as an IPv6 address.
	if family == attribute_FAMILY_IPV4 {
		xorIP = xorIP[:4]
	}
	return &Host{family, net.IP(xorIP).String(), port ^ (magicCookie >> 16)}
}

func (v *attribute) address() *Host {
	host := new(Host)
	host.family = binary.BigEndian.Uint16(v.value[0:2])
	host.port = binary.BigEndian.Uint16(v.value[2:4])

	// Truncate if IPv4, otherwise net.IP sometimes renders it as an IPv6 address.
	if host.family == attribute_FAMILY_IPV4 {
		v.value = v.value[:8]
	}
	host.ip = net.IP(v.value[4:]).String()
	return host
}
