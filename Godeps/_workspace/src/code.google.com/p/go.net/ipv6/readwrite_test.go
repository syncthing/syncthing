// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6_test

import (
	"bytes"
	"code.google.com/p/go.net/ipv6"
	"net"
	"runtime"
	"sync"
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
	if n, err := p.WriteTo(wb, &cm, dst); err != nil {
		b.Fatalf("ipv6.PacketConn.WriteTo failed: %v", err)
	} else if n != len(wb) {
		b.Fatalf("ipv6.PacketConn.WriteTo failed: short write: %v", n)
	}
	if _, _, _, err := p.ReadFrom(rb); err != nil {
		b.Fatalf("ipv6.PacketConn.ReadFrom failed: %v", err)
	}
}

func TestPacketConnConcurrentReadWriteUnicastUDP(t *testing.T) {
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

	ifi := loopbackInterface()
	cf := ipv6.FlagTrafficClass | ipv6.FlagHopLimit | ipv6.FlagSrc | ipv6.FlagDst | ipv6.FlagInterface | ipv6.FlagPathMTU
	wb := []byte("HELLO-R-U-THERE")

	var wg sync.WaitGroup
	reader := func() {
		defer wg.Done()
		rb := make([]byte, 128)
		if n, cm, _, err := p.ReadFrom(rb); err != nil {
			t.Errorf("ipv6.PacketConn.ReadFrom failed: %v", err)
			return
		} else if !bytes.Equal(rb[:n], wb) {
			t.Errorf("got %v; expected %v", rb[:n], wb)
			return
		} else {
			t.Logf("rcvd cmsg: %v", cm)
		}
	}
	writer := func(toggle bool) {
		defer wg.Done()
		cm := ipv6.ControlMessage{
			TrafficClass: DiffServAF11 | CongestionExperienced,
			Src:          net.IPv6loopback,
			Dst:          net.IPv6loopback,
		}
		if ifi != nil {
			cm.IfIndex = ifi.Index
		}
		if err := p.SetControlMessage(cf, toggle); err != nil {
			t.Errorf("ipv6.PacketConn.SetControlMessage failed: %v", err)
			return
		}
		if n, err := p.WriteTo(wb, &cm, dst); err != nil {
			t.Errorf("ipv6.PacketConn.WriteTo failed: %v", err)
			return
		} else if n != len(wb) {
			t.Errorf("ipv6.PacketConn.WriteTo failed: short write: %v", n)
			return
		}
	}

	const N = 10
	wg.Add(N)
	for i := 0; i < N; i++ {
		go reader()
	}
	wg.Add(2 * N)
	for i := 0; i < 2*N; i++ {
		go writer(i%2 != 0)
	}
	wg.Add(N)
	for i := 0; i < N; i++ {
		go reader()
	}
	wg.Wait()
}
