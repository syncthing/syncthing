// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6_test

import (
	"code.google.com/p/go.net/ipv6"
	"fmt"
	"net"
	"os"
	"runtime"
	"testing"
)

var udpMultipleGroupListenerTests = []net.Addr{
	&net.UDPAddr{IP: net.ParseIP("ff02::114")}, // see RFC 4727
	&net.UDPAddr{IP: net.ParseIP("ff02::1:114")},
	&net.UDPAddr{IP: net.ParseIP("ff02::2:114")},
}

func TestUDPSinglePacketConnWithMultipleGroupListeners(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	for _, gaddr := range udpMultipleGroupListenerTests {
		c, err := net.ListenPacket("udp6", "[::]:0") // wildcard address with non-reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c.Close()

		p := ipv6.NewPacketConn(c)
		var mift []*net.Interface

		ift, err := net.Interfaces()
		if err != nil {
			t.Fatalf("net.Interfaces failed: %v", err)
		}
		for i, ifi := range ift {
			if _, ok := isMulticastAvailable(&ifi); !ok {
				continue
			}
			if err := p.JoinGroup(&ifi, gaddr); err != nil {
				t.Fatalf("ipv6.PacketConn.JoinGroup %v on %v failed: %v", gaddr, ifi, err)
			}
			mift = append(mift, &ift[i])
		}
		for _, ifi := range mift {
			if err := p.LeaveGroup(ifi, gaddr); err != nil {
				t.Fatalf("ipv6.PacketConn.LeaveGroup %v on %v failed: %v", gaddr, ifi, err)
			}
		}
	}
}

func TestUDPMultipleConnWithMultipleGroupListeners(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	for _, gaddr := range udpMultipleGroupListenerTests {
		c1, err := net.ListenPacket("udp6", "[ff02::]:1024") // wildcard address with reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c1.Close()

		c2, err := net.ListenPacket("udp6", "[ff02::]:1024") // wildcard address with reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c2.Close()

		var ps [2]*ipv6.PacketConn
		ps[0] = ipv6.NewPacketConn(c1)
		ps[1] = ipv6.NewPacketConn(c2)
		var mift []*net.Interface

		ift, err := net.Interfaces()
		if err != nil {
			t.Fatalf("net.Interfaces failed: %v", err)
		}
		for i, ifi := range ift {
			if _, ok := isMulticastAvailable(&ifi); !ok {
				continue
			}
			for _, p := range ps {
				if err := p.JoinGroup(&ifi, gaddr); err != nil {
					t.Fatalf("ipv6.PacketConn.JoinGroup %v on %v failed: %v", gaddr, ifi, err)
				}
			}
			mift = append(mift, &ift[i])
		}
		for _, ifi := range mift {
			for _, p := range ps {
				if err := p.LeaveGroup(ifi, gaddr); err != nil {
					t.Fatalf("ipv6.PacketConn.LeaveGroup %v on %v failed: %v", gaddr, ifi, err)
				}
			}
		}
	}
}

func TestUDPPerInterfaceSinglePacketConnWithSingleGroupListener(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	gaddr := &net.IPAddr{IP: net.ParseIP("ff02::114")} // see RFC 4727
	type ml struct {
		c   *ipv6.PacketConn
		ifi *net.Interface
	}
	var mlt []*ml

	ift, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces failed: %v", err)
	}
	for i, ifi := range ift {
		ip, ok := isMulticastAvailable(&ifi)
		if !ok {
			continue
		}
		c, err := net.ListenPacket("udp6", fmt.Sprintf("[%s%%%s]:1024", ip.String(), ifi.Name)) // unicast address with non-reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket with %v failed: %v", ip, err)
		}
		defer c.Close()
		p := ipv6.NewPacketConn(c)
		if err := p.JoinGroup(&ifi, gaddr); err != nil {
			t.Fatalf("ipv6.PacketConn.JoinGroup on %v failed: %v", ifi, err)
		}
		mlt = append(mlt, &ml{p, &ift[i]})
	}
	for _, m := range mlt {
		if err := m.c.LeaveGroup(m.ifi, gaddr); err != nil {
			t.Fatalf("ipv6.PacketConn.LeaveGroup on %v failed: %v", m.ifi, err)
		}
	}
}

func TestIPSinglePacketConnWithSingleGroupListener(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}

	c, err := net.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	p := ipv6.NewPacketConn(c)
	gaddr := &net.IPAddr{IP: net.ParseIP("ff02::114")} // see RFC 4727
	var mift []*net.Interface

	ift, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces failed: %v", err)
	}
	for i, ifi := range ift {
		if _, ok := isMulticastAvailable(&ifi); !ok {
			continue
		}
		if err := p.JoinGroup(&ifi, gaddr); err != nil {
			t.Fatalf("ipv6.PacketConn.JoinGroup on %v failed: %v", ifi, err)
		}
		mift = append(mift, &ift[i])
	}
	for _, ifi := range mift {
		if err := p.LeaveGroup(ifi, gaddr); err != nil {
			t.Fatalf("ipv6.PacketConn.LeaveGroup on %v failed: %v", ifi, err)
		}
	}
}
