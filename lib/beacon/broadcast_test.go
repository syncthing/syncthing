// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"net"
	"testing"
)

var addrToBcast = []struct {
	addr, bcast string
}{
	{"172.16.32.33/25", "172.16.32.127/25"},
	{"172.16.32.129/25", "172.16.32.255/25"},
	{"172.16.32.33/24", "172.16.32.255/24"},
	{"172.16.32.33/22", "172.16.35.255/22"},
	{"172.16.32.33/0", "255.255.255.255/0"},
	{"172.16.32.33/32", "172.16.32.33/32"},
}

func TestBroadcastAddr(t *testing.T) {
	for _, tc := range addrToBcast {
		_, net, err := net.ParseCIDR(tc.addr)
		if err != nil {
			t.Fatal(err)
		}
		bc := bcast(net).String()
		if bc != tc.bcast {
			t.Errorf("%q != %q", bc, tc.bcast)
		}
	}
}
