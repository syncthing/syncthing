// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6_test

import (
	"code.google.com/p/go.net/ipv6"
	"net"
	"os"
	"runtime"
	"testing"
)

var packetConnMulticastSocketOptionTests = []struct {
	net, proto, addr string
	gaddr            net.Addr
}{
	{"udp6", "", "[ff02::]:0", &net.UDPAddr{IP: net.ParseIP("ff02::114")}}, // see RFC 4727
	{"ip6", ":ipv6-icmp", "::", &net.IPAddr{IP: net.ParseIP("ff02::114")}}, // see RFC 4727
}

func TestPacketConnMulticastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}
	ifi := loopbackInterface()
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	for _, tt := range packetConnMulticastSocketOptionTests {
		if tt.net == "ip6" && os.Getuid() != 0 {
			t.Skip("must be root")
		}
		c, err := net.ListenPacket(tt.net+tt.proto, tt.addr)
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c.Close()

		p := ipv6.NewPacketConn(c)

		hoplim := 255
		if err := p.SetMulticastHopLimit(hoplim); err != nil {
			t.Fatalf("ipv6.PacketConn.SetMulticastHopLimit failed: %v", err)
		}
		if v, err := p.MulticastHopLimit(); err != nil {
			t.Fatalf("ipv6.PacketConn.MulticastHopLimit failed: %v", err)
		} else if v != hoplim {
			t.Fatalf("got unexpected multicast hop limit %v; expected %v", v, hoplim)
		}

		for _, toggle := range []bool{true, false} {
			if err := p.SetMulticastLoopback(toggle); err != nil {
				t.Fatalf("ipv6.PacketConn.SetMulticastLoopback failed: %v", err)
			}
			if v, err := p.MulticastLoopback(); err != nil {
				t.Fatalf("ipv6.PacketConn.MulticastLoopback failed: %v", err)
			} else if v != toggle {
				t.Fatalf("got unexpected multicast loopback %v; expected %v", v, toggle)
			}
		}

		if err := p.JoinGroup(ifi, tt.gaddr); err != nil {
			t.Fatalf("ipv6.PacketConn.JoinGroup(%v, %v) failed: %v", ifi, tt.gaddr, err)
		}
		if err := p.LeaveGroup(ifi, tt.gaddr); err != nil {
			t.Fatalf("ipv6.PacketConn.LeaveGroup(%v, %v) failed: %v", ifi, tt.gaddr, err)
		}
	}
}
