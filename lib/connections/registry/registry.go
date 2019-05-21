// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Registry tracks connections on which we are listening on, to allow us to pick a connection that has a NAT port
// mapping. This also makes our outgoing port stable and same as incoming port which should allow
// better probability of punching through.
package registry

import (
	"net"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/sync"
)

var (
	mut            = sync.NewMutex()
	availableConns = make(map[string][]net.Conn)
)

func Register(scheme string, conn net.Conn) {
	mut.Lock()
	defer mut.Unlock()

	availableConns[scheme] = append(availableConns[scheme], conn)
}

func Unregister(scheme string, conn net.Conn) {
	mut.Lock()
	defer mut.Unlock()

	conns := availableConns[scheme]
	for i, f := range conns {
		if f == conn {
			copy(conns[i:], conns[i+1:])
			conns[len(conns)-1] = nil
			availableConns[scheme] = conns[:len(conns)-1]
			break
		}
	}
}

func Get(scheme string) net.Conn {
	mut.Lock()
	defer mut.Unlock()

	candidates := make([]net.Conn, 0)
	for availableScheme, conns := range availableConns {
		// quic:// should be considered ok for both quic4:// and quic6://
		if strings.HasPrefix(scheme, availableScheme) {
			candidates = append(candidates, conns...)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, connSorter(candidates))
	return candidates[0]
}

// Sort connections by whether they are unspecified or not, as connections
// listening on all addresses are more useful.
func connSorter(conns []net.Conn) func(int, int) bool {
	return func(i, j int) bool {
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
}
