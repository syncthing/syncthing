// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stun

import (
	"bytes"
	"net"
	"sync"
	"time"
)

const (
	stunFilterPriority = 10
	otherDataPriority  = 100
)

type stunFilter struct {
	ids map[string]time.Time
	mut sync.Mutex
}

func (f *stunFilter) Outgoing(out []byte, addr net.Addr) {
	if !f.isStunPayload(out) {
		panic("not a stun payload")
	}
	f.mut.Lock()
	f.ids[string(out[8:20])] = time.Now().Add(time.Minute)
	f.reap()
	f.mut.Unlock()
}

func (f *stunFilter) ClaimIncoming(in []byte, addr net.Addr) bool {
	if f.isStunPayload(in) {
		f.mut.Lock()
		_, ok := f.ids[string(in[8:20])]
		f.reap()
		f.mut.Unlock()
		return ok
	}
	return false
}

func (*stunFilter) isStunPayload(data []byte) bool {
	// Need at least 20 bytes
	if len(data) < 20 {
		return false
	}

	// First two bits always unset, and should always send magic cookie.
	return data[0]&0xc0 == 0 && bytes.Equal(data[4:8], []byte{0x21, 0x12, 0xA4, 0x42})
}

func (f *stunFilter) reap() {
	now := time.Now()
	for id, timeout := range f.ids {
		if timeout.Before(now) {
			delete(f.ids, id)
		}
	}
}
