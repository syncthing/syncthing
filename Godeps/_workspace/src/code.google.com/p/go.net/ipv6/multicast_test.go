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

func TestPacketConnReadWriteMulticastUDP(t *testing.T) {
	switch runtime.GOOS {
	case "freebsd": // due to a bug on loopback marking
		// See http://www.freebsd.org/cgi/query-pr.cgi?pr=180065.
		t.Skipf("not supported on %q", runtime.GOOS)
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

	c, err := net.ListenPacket("udp6", "[ff02::114]:0") // see RFC 4727
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	_, port, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort failed: %v", err)
	}
	dst, err := net.ResolveUDPAddr("udp6", "[ff02::114]:"+port) // see RFC 4727
	if err != nil {
		t.Fatalf("net.ResolveUDPAddr failed: %v", err)
	}

	p := ipv6.NewPacketConn(c)
	if err := p.JoinGroup(ifi, dst); err != nil {
		t.Fatalf("ipv6.PacketConn.JoinGroup on %v failed: %v", ifi, err)
	}
	if err := p.SetMulticastInterface(ifi); err != nil {
		t.Fatalf("ipv6.PacketConn.SetMulticastInterface failed: %v", err)
	}
	if err := p.SetMulticastLoopback(true); err != nil {
		t.Fatalf("ipv6.PacketConn.SetMulticastLoopback failed: %v", err)
	}

	cm := ipv6.ControlMessage{
		TrafficClass: DiffServAF11 | CongestionExperienced,
		IfIndex:      ifi.Index,
	}
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagInterface | ipv6.FlagPathMTU

	for i, toggle := range []bool{true, false, true} {
		if err := p.SetControlMessage(cf, toggle); err != nil {
			t.Fatalf("ipv6.PacketConn.SetControlMessage failed: %v", err)
		}
		cm.HopLimit = i + 1
		if _, err := p.WriteTo([]byte("HELLO-R-U-THERE"), &cm, dst); err != nil {
			t.Fatalf("ipv6.PacketConn.WriteTo failed: %v", err)
		}
		b := make([]byte, 128)
		if _, cm, _, err := p.ReadFrom(b); err != nil {
			t.Fatalf("ipv6.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
		}
	}
}

func TestPacketConnReadWriteMulticastICMP(t *testing.T) {
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
	ifi := loopbackInterface()
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip6", "ff02::114") // see RFC 4727
	if err != nil {
		t.Fatalf("net.ResolveIPAddr failed: %v", err)
	}

	p := ipv6.NewPacketConn(c)
	if err := p.JoinGroup(ifi, dst); err != nil {
		t.Fatalf("ipv6.PacketConn.JoinGroup on %v failed: %v", ifi, err)
	}
	if err := p.SetMulticastInterface(ifi); err != nil {
		t.Fatalf("ipv6.PacketConn.SetMulticastInterface failed: %v", err)
	}
	if err := p.SetMulticastLoopback(true); err != nil {
		t.Fatalf("ipv6.PacketConn.SetMulticastLoopback failed: %v", err)
	}

	cm := ipv6.ControlMessage{
		TrafficClass: DiffServAF11 | CongestionExperienced,
		IfIndex:      ifi.Index,
	}
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagInterface | ipv6.FlagPathMTU

	var f ipv6.ICMPFilter
	f.SetAll(true)
	f.Set(ipv6.ICMPTypeEchoReply, false)
	if err := p.SetICMPFilter(&f); err != nil {
		t.Fatalf("ipv6.PacketConn.SetICMPFilter failed: %v", err)
	}

	for i, toggle := range []bool{true, false, true} {
		wb, err := (&icmpMessage{
			Type: ipv6.ICMPTypeEchoRequest, Code: 0,
			Body: &icmpEcho{
				ID: os.Getpid() & 0xffff, Seq: i + 1,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}).Marshal()
		if err != nil {
			t.Fatalf("icmpMessage.Marshal failed: %v", err)
		}
		if err := p.SetControlMessage(cf, toggle); err != nil {
			t.Fatalf("ipv6.PacketConn.SetControlMessage failed: %v", err)
		}
		cm.HopLimit = i + 1
		if _, err := p.WriteTo(wb, &cm, dst); err != nil {
			t.Fatalf("ipv6.PacketConn.WriteTo failed: %v", err)
		}
		b := make([]byte, 128)
		if n, cm, _, err := p.ReadFrom(b); err != nil {
			t.Fatalf("ipv6.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
			if m, err := parseICMPMessage(b[:n]); err != nil {
				t.Fatalf("parseICMPMessage failed: %v", err)
			} else if m.Type != ipv6.ICMPTypeEchoReply || m.Code != 0 {
				t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, ipv6.ICMPTypeEchoReply, 0)
			}
		}
	}
}
