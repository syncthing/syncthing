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

var supportsIPv6 bool

func init() {
	if ln, err := net.Listen("tcp6", "[::1]:0"); err == nil {
		ln.Close()
		supportsIPv6 = true
	}
}

var condFatalf = func() func(*testing.T, string, ...interface{}) {
	// A few APIs are not implemented yet on some platforms.
	switch runtime.GOOS {
	case "darwin", "plan9", "windows":
		return (*testing.T).Logf
	}
	return (*testing.T).Fatalf
}()

func TestConnInitiatorPathMTU(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	done := make(chan bool)
	go acceptor(t, ln, done)

	c, err := net.Dial("tcp6", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer c.Close()

	if pmtu, err := ipv6.NewConn(c).PathMTU(); err != nil {
		condFatalf(t, "ipv6.Conn.PathMTU failed: %v", err)
	} else {
		t.Logf("path mtu for %v: %v", c.RemoteAddr(), pmtu)
	}

	<-done
}

func TestConnResponderPathMTU(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	done := make(chan bool)
	go connector(t, "tcp6", ln.Addr().String(), done)

	c, err := ln.Accept()
	if err != nil {
		t.Fatalf("net.Accept failed: %v", err)
	}
	defer c.Close()

	if pmtu, err := ipv6.NewConn(c).PathMTU(); err != nil {
		condFatalf(t, "ipv6.Conn.PathMTU failed: %v", err)
	} else {
		t.Logf("path mtu for %v: %v", c.RemoteAddr(), pmtu)
	}

	<-done
}

func TestPacketConnChecksum(t *testing.T) {
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

	c, err := net.ListenPacket("ip6:89", "::") // OSPF for IPv6
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	p := ipv6.NewPacketConn(c)
	offset := 12 // see RFC 5340

	for _, toggle := range []bool{false, true} {
		if err := p.SetChecksum(toggle, offset); err != nil {
			if toggle {
				t.Fatalf("ipv6.PacketConn.SetChecksum(%v, %v) failed: %v", toggle, offset, err)
			} else {
				// Some platforms never allow to disable the kernel
				// checksum processing.
				t.Logf("ipv6.PacketConn.SetChecksum(%v, %v) failed: %v", toggle, offset, err)
			}
		}
		if on, offset, err := p.Checksum(); err != nil {
			t.Fatalf("ipv6.PacketConn.Checksum failed: %v", err)
		} else {
			t.Logf("kernel checksum processing enabled=%v, offset=%v", on, offset)
		}
	}
}
