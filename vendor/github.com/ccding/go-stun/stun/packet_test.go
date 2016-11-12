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
	"testing"
)

func TestNewPacketFromBytes(t *testing.T) {
	b := make([]byte, 23)
	_, err := newPacketFromBytes(b)
	if err == nil {
		t.Errorf("newPacketFromBytes error")
	}
	b = make([]byte, 24)
	_, err = newPacketFromBytes(b)
	if err != nil {
		t.Errorf("newPacketFromBytes error")
	}
}

func TestNewPacket(t *testing.T) {
	_, err := newPacket()
	if err != nil {
		t.Errorf("newPacket error")
	}
}

func TestPacketAll(t *testing.T) {
	p, err := newPacket()
	if err != nil {
		t.Errorf("newPacket error")
	}
	p.addAttribute(*newChangeReqAttribute(true, true))
	p.addAttribute(*newSoftwareAttribute("aaa"))
	p.addAttribute(*newFingerprintAttribute(p))
	pkt, err := newPacketFromBytes(p.bytes())
	if err != nil {
		t.Errorf("newPacketFromBytes error")
	}
	if pkt.types != 0 {
		t.Errorf("newPacketFromBytes error")
	}
	if pkt.length < 24 {
		t.Errorf("newPacketFromBytes error")
	}
}
