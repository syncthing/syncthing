// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6_test

import (
	"bytes"
	"code.google.com/p/go.net/ipv6"
	"net"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestPacketConnReadWriteUnicastUDP(t *testing.T) {
	switch runtime.GOOS {
	case "dragonfly", "plan9", "solaris", "windows":
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
	p := ipv6.NewPacketConn(c)
	defer p.Close()

	dst, err := net.ResolveUDPAddr("udp6", c.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.ResolveUDPAddr failed: %v", err)
	}

	cm := ipv6.ControlMessage{
		TrafficClass: DiffServAF11 | CongestionExperienced,
		Src:          net.IPv6loopback,
		Dst:          net.IPv6loopback,
	}
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagSrc | ipv6.FlagDst | ipv6.FlagInterface | ipv6.FlagPathMTU
	ifi := loopbackInterface()
	if ifi != nil {
		cm.IfIndex = ifi.Index
	}
	wb := []byte("HELLO-R-U-THERE")

	for i, toggle := range []bool{true, false, true} {
		if err := p.SetControlMessage(cf, toggle); err != nil {
			t.Fatalf("ipv6.PacketConn.SetControlMessage failed: %v", err)
		}
		cm.HopLimit = i + 1
		if err := p.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv6.PacketConn.SetWriteDeadline failed: %v", err)
		}
		if n, err := p.WriteTo(wb, &cm, dst); err != nil {
			t.Fatalf("ipv6.PacketConn.WriteTo failed: %v", err)
		} else if n != len(wb) {
			t.Fatalf("ipv6.PacketConn.WriteTo failed: short write: %v", n)
		}
		rb := make([]byte, 128)
		if err := p.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv6.PacketConn.SetReadDeadline failed: %v", err)
		}
		if n, cm, _, err := p.ReadFrom(rb); err != nil {
			t.Fatalf("ipv6.PacketConn.ReadFrom failed: %v", err)
		} else if !bytes.Equal(rb[:n], wb) {
			t.Fatalf("got %v; expected %v", rb[:n], wb)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
		}
	}
}

func TestPacketConnReadWriteUnicastICMP(t *testing.T) {
	switch runtime.GOOS {
	case "dragonfly", "plan9", "solaris", "windows":
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
	p := ipv6.NewPacketConn(c)
	defer p.Close()

	dst, err := net.ResolveIPAddr("ip6", "::1")
	if err != nil {
		t.Fatalf("net.ResolveIPAddr failed: %v", err)
	}

	pshicmp := ipv6PseudoHeader(c.LocalAddr().(*net.IPAddr).IP, dst.IP, ianaProtocolIPv6ICMP)
	cm := ipv6.ControlMessage{
		TrafficClass: DiffServAF11 | CongestionExperienced,
		Src:          net.IPv6loopback,
		Dst:          net.IPv6loopback,
	}
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagSrc | ipv6.FlagDst | ipv6.FlagInterface | ipv6.FlagPathMTU
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

	var psh []byte
	for i, toggle := range []bool{true, false, true} {
		if toggle {
			psh = nil
			if err := p.SetChecksum(true, 2); err != nil {
				t.Fatalf("ipv6.PacketConn.SetChecksum failed: %v", err)
			}
		} else {
			psh = pshicmp
			// Some platforms never allow to disable the
			// kernel checksum processing.
			p.SetChecksum(false, -1)
		}
		wb, err := (&icmpMessage{
			Type: ipv6.ICMPTypeEchoRequest, Code: 0,
			Body: &icmpEcho{
				ID: os.Getpid() & 0xffff, Seq: i + 1,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}).Marshal(psh)
		if err != nil {
			t.Fatalf("icmpMessage.Marshal failed: %v", err)
		}
		if err := p.SetControlMessage(cf, toggle); err != nil {
			t.Fatalf("ipv6.PacketConn.SetControlMessage failed: %v", err)
		}
		cm.HopLimit = i + 1
		if err := p.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv6.PacketConn.SetWriteDeadline failed: %v", err)
		}
		if n, err := p.WriteTo(wb, &cm, dst); err != nil {
			t.Fatalf("ipv6.PacketConn.WriteTo failed: %v", err)
		} else if n != len(wb) {
			t.Fatalf("ipv6.PacketConn.WriteTo failed: short write: %v", n)
		}
		rb := make([]byte, 128)
		if err := p.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv6.PacketConn.SetReadDeadline failed: %v", err)
		}
		if n, cm, _, err := p.ReadFrom(rb); err != nil {
			t.Fatalf("ipv6.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
			if m, err := parseICMPMessage(rb[:n]); err != nil {
				t.Fatalf("parseICMPMessage failed: %v", err)
			} else if m.Type != ipv6.ICMPTypeEchoReply || m.Code != 0 {
				t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, ipv6.ICMPTypeEchoReply, 0)
			}
		}
	}
}
