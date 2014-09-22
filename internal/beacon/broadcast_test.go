// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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
