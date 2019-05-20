// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"bytes"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
)

var (
	mut   sync.Mutex
	conns []net.PacketConn
)

var (
	stunFilterPriority = 10
	quicFilterPriority = 100
	quicConfig         = &quic.Config{
		ConnectionIDLength: 4,
		KeepAlive:          true,
	}
)

// Track connections on which we are listening on, to increase the probability of accessing a connection that
// has a NAT port mapping. This also makes our outgoing port stable and same as incoming port which should allow
// better probability of punching through.
func getListeningConn() net.PacketConn {
	mut.Lock()
	defer mut.Unlock()
	if len(conns) == 0 {
		return nil
	}
	return conns[0]
}

func registerConn(conn net.PacketConn) {
	mut.Lock()
	defer mut.Unlock()
	conns = append(conns, conn)

	sort.Slice(conns, connSort)
}

func deregisterConn(conn net.PacketConn) {
	mut.Lock()
	defer mut.Unlock()

	for i, f := range conns {
		if f == conn {
			copy(conns[i:], conns[i+1:])
			conns[len(conns)-1] = nil
			conns = conns[:len(conns)-1]
			break
		}
	}
	sort.Slice(conns, connSort)
}

// Sort connections by whether they are unspecified or not, as connections
// listening on all addresses are more useful.
func connSort(i, j int) bool {
	iIsUnspecified := false
	jIsUnspecified := false
	if host, _, err := net.SplitHostPort(conns[i].LocalAddr().String()); err == nil {
		iIsUnspecified = net.ParseIP(host).IsUnspecified()
	}
	if host, _, err := net.SplitHostPort(conns[j].LocalAddr().String()); err == nil {
		jIsUnspecified = net.ParseIP(host).IsUnspecified()
	}
	return (iIsUnspecified && !jIsUnspecified) || (iIsUnspecified && jIsUnspecified)
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
