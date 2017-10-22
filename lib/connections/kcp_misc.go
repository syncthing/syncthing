// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"bytes"
	"encoding/binary"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AudriusButkevicius/kcp-go"
	"github.com/AudriusButkevicius/pfilter"
	"github.com/xtaci/smux"
)

var (
	mut     sync.Mutex
	filters filterList
)

func init() {
	kcp.BlacklistDuration = 10 * time.Minute
}

type filterList []*pfilter.PacketFilter

// Sort connections by whether they are unspecified or not, as connections
// listening on all addresses are more useful.
func (f filterList) Len() int      { return len(f) }
func (f filterList) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f filterList) Less(i, j int) bool {
	iIsUnspecified := false
	jIsUnspecified := false
	if host, _, err := net.SplitHostPort(f[i].LocalAddr().String()); err == nil {
		iIsUnspecified = net.ParseIP(host).IsUnspecified()
	}
	if host, _, err := net.SplitHostPort(f[j].LocalAddr().String()); err == nil {
		jIsUnspecified = net.ParseIP(host).IsUnspecified()
	}
	return (iIsUnspecified && !jIsUnspecified) || (iIsUnspecified && jIsUnspecified)
}

// As we open listen KCP connections, we register them here, so that Dial calls through
// KCP could reuse them. This way we will hopefully work around restricted NATs by
// dialing via the same connection we are listening on, creating a mapping on our NAT
// to that IP, and hoping that the other end will try to dial our listen address and
// using the mapping we've established when we dialed.
func getDialingFilter() *pfilter.PacketFilter {
	mut.Lock()
	defer mut.Unlock()
	if len(filters) == 0 {
		return nil
	}
	return filters[0]
}

func registerFilter(filter *pfilter.PacketFilter) {
	mut.Lock()
	defer mut.Unlock()
	filters = append(filters, filter)
	sort.Sort(filterList(filters))
}

func deregisterFilter(filter *pfilter.PacketFilter) {
	mut.Lock()
	defer mut.Unlock()

	for i, f := range filters {
		if f == filter {
			copy(filters[i:], filters[i+1:])
			filters[len(filters)-1] = nil
			filters = filters[:len(filters)-1]
			break
		}
	}
	sort.Sort(filterList(filters))
}

// Filters

type kcpConversationFilter struct {
	convID uint32
}

func (f *kcpConversationFilter) Outgoing(out []byte, addr net.Addr) {
	if !f.isKCPConv(out) {
		panic("not a kcp conversation")
	}
	atomic.StoreUint32(&f.convID, binary.LittleEndian.Uint32(out[:4]))
}

func (kcpConversationFilter) isKCPConv(data []byte) bool {
	// Need at least 5 bytes
	if len(data) < 5 {
		return false
	}

	// First 4 bytes convID
	// 5th byte is cmd
	// IKCP_CMD_PUSH    = 81 // cmd: push data
	// IKCP_CMD_ACK     = 82 // cmd: ack
	// IKCP_CMD_WASK    = 83 // cmd: window probe (ask)
	// IKCP_CMD_WINS    = 84 // cmd: window size (tell)
	return 80 < data[4] && data[4] < 85
}

func (f *kcpConversationFilter) ClaimIncoming(in []byte, addr net.Addr) bool {
	if f.isKCPConv(in) {
		convID := atomic.LoadUint32(&f.convID)
		return convID != 0 && binary.LittleEndian.Uint32(in[:4]) == convID
	}
	return false
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

type sessionClosingStream struct {
	*smux.Stream
	session *smux.Session
}

func (w *sessionClosingStream) Close() error {
	err1 := w.Stream.Close()

	deadline := time.Now().Add(5 * time.Second)
	for w.session.NumStreams() > 0 && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}

	err2 := w.session.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
