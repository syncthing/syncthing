// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.12

package connections

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucas-clemente/quic-go"
)

var (
	stunFilterPriority = 10
	quicFilterPriority = 100
	quicConfig         = &quic.Config{
		ConnectionIDLength: 4,
		KeepAlive:          true,
	}
)

type quicTlsConn struct {
	quic.Session
	quic.Stream
}

func (q *quicTlsConn) Close() error {
	sterr := q.Stream.Close()
	seerr := q.Session.Close()
	if sterr != nil {
		return sterr
	}
	return seerr
}

// Sort available packet connections by ip address, preferring unspecified local address.
func packetConnLess(i interface{}, j interface{}) bool {
	iIsUnspecified := false
	jIsUnspecified := false
	iLocalAddr := i.(net.PacketConn).LocalAddr()
	jLocalAddr := j.(net.PacketConn).LocalAddr()

	if host, _, err := net.SplitHostPort(iLocalAddr.String()); err == nil {
		iIsUnspecified = host == "" || net.ParseIP(host).IsUnspecified()
	}
	if host, _, err := net.SplitHostPort(jLocalAddr.String()); err == nil {
		jIsUnspecified = host == "" || net.ParseIP(host).IsUnspecified()
	}

	if jIsUnspecified != iIsUnspecified {
		return len(iLocalAddr.Network()) <= len(jLocalAddr.Network())
	}

	return iIsUnspecified
}

type writeTrackingPacketConn struct {
	net.PacketConn
	lastWrite atomic.Value
}

func (c *writeTrackingPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	c.lastWrite.Store(time.Now())
	return c.PacketConn.WriteTo(p, addr)
}

func (c *writeTrackingPacketConn) GetLastWrite() time.Time {
	return c.lastWrite.Load().(time.Time)
}

type stunFilter struct {
	ids map[string]time.Time
	mut sync.Mutex
}

func (f *stunFilter) Outgoing(out []byte, addr net.Addr) {
	if !f.isStunPayload(out) {
		panic("not a stun payload")
	}
	id := string(out[8:20])
	f.mut.Lock()
	f.ids[id] = time.Now().Add(time.Minute)
	f.reap()
	f.mut.Unlock()
}

func (f *stunFilter) ClaimIncoming(in []byte, addr net.Addr) bool {
	if f.isStunPayload(in) {
		id := string(in[8:20])
		f.mut.Lock()
		_, ok := f.ids[id]
		f.reap()
		f.mut.Unlock()
		return ok
	}
	return false
}

func (f *stunFilter) isStunPayload(data []byte) bool {
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
