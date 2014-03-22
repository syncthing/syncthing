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

func benchmarkUDPListener() (net.PacketConn, net.Addr, error) {
	c, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		return nil, nil, err
	}
	dst, err := net.ResolveUDPAddr("udp6", c.LocalAddr().String())
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	return c, dst, nil
}

func BenchmarkReadWriteNetUDP(b *testing.B) {
	c, dst, err := benchmarkUDPListener()
	if err != nil {
		b.Fatalf("benchmarkUDPListener failed: %v", err)
	}
	defer c.Close()

	wb, rb := []byte("HELLO-R-U-THERE"), make([]byte, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkReadWriteNetUDP(b, c, wb, rb, dst)
	}
}

func benchmarkReadWriteNetUDP(b *testing.B, c net.PacketConn, wb, rb []byte, dst net.Addr) {
	if _, err := c.WriteTo(wb, dst); err != nil {
		b.Fatalf("net.PacketConn.WriteTo failed: %v", err)
	}
	if _, _, err := c.ReadFrom(rb); err != nil {
		b.Fatalf("net.PacketConn.ReadFrom failed: %v", err)
	}
}

func BenchmarkReadWriteIPv6UDP(b *testing.B) {
	c, dst, err := benchmarkUDPListener()
	if err != nil {
		b.Fatalf("benchmarkUDPListener failed: %v", err)
	}
	defer c.Close()

	p := ipv6.NewPacketConn(c)
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagInterface | ipv6.FlagPathMTU
	if err := p.SetControlMessage(cf, true); err != nil {
		b.Fatalf("ipv6.PacketConn.SetControlMessage failed: %v", err)
	}
	ifi := loopbackInterface()

	wb, rb := []byte("HELLO-R-U-THERE"), make([]byte, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkReadWriteIPv6UDP(b, p, wb, rb, dst, ifi)
	}
}

func benchmarkReadWriteIPv6UDP(b *testing.B, p *ipv6.PacketConn, wb, rb []byte, dst net.Addr, ifi *net.Interface) {
	cm := ipv6.ControlMessage{
		TrafficClass: DiffServAF11 | CongestionExperienced,
		HopLimit:     1,
	}
	if ifi != nil {
		cm.IfIndex = ifi.Index
	}
	if _, err := p.WriteTo(wb, &cm, dst); err != nil {
		b.Fatalf("ipv6.PacketConn.WriteTo failed: %v", err)
	}
	if _, _, _, err := p.ReadFrom(rb); err != nil {
		b.Fatalf("ipv6.PacketConn.ReadFrom failed: %v", err)
	}
}

func TestPacketConnReadWriteUnicastUDP(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	c, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	dst, err := net.ResolveUDPAddr("udp6", c.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.ResolveUDPAddr failed: %v", err)
	}

	p := ipv6.NewPacketConn(c)
	cm := ipv6.ControlMessage{
		TrafficClass: DiffServAF11 | CongestionExperienced,
	}
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagInterface | ipv6.FlagPathMTU
	ifi := loopbackInterface()
	if ifi != nil {
		cm.IfIndex = ifi.Index
	}

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

func TestPacketConnReadWriteUnicastICMP(t *testing.T) {
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

	c, err := net.ListenPacket("ip6:ipv6-icmp", "::1")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip6", "::1")
	if err != nil {
		t.Fatalf("net.ResolveIPAddr failed: %v", err)
	}

	p := ipv6.NewPacketConn(c)
	cm := ipv6.ControlMessage{TrafficClass: DiffServAF11 | CongestionExperienced}
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagInterface | ipv6.FlagPathMTU
	ifi := loopbackInterface()
	if ifi != nil {
		cm.IfIndex = ifi.Index
	}

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
