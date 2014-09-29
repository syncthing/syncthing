// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
