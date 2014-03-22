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

func TestConnUnicastSocketOptions(t *testing.T) {
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

	testUnicastSocketOptions(t, ipv6.NewConn(c))

	<-done
}

var packetConnUnicastSocketOptionTests = []struct {
	net, proto, addr string
}{
	{"udp6", "", "[::1]:0"},
	{"ip6", ":ipv6-icmp", "::1"},
}

func TestPacketConnUnicastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if !supportsIPv6 {
		t.Skip("ipv6 is not supported")
	}

	for _, tt := range packetConnUnicastSocketOptionTests {
		if tt.net == "ip6" && os.Getuid() != 0 {
			t.Skip("must be root")
		}
		c, err := net.ListenPacket(tt.net+tt.proto, tt.addr)
		if err != nil {
			t.Fatalf("net.ListenPacket(%q, %q) failed: %v", tt.net+tt.proto, tt.addr, err)
		}
		defer c.Close()

		testUnicastSocketOptions(t, ipv6.NewPacketConn(c))
	}
}

type testIPv6UnicastConn interface {
	TrafficClass() (int, error)
	SetTrafficClass(int) error
	HopLimit() (int, error)
	SetHopLimit(int) error
}

func testUnicastSocketOptions(t *testing.T, c testIPv6UnicastConn) {
	tclass := DiffServCS0 | NotECNTransport
	if err := c.SetTrafficClass(tclass); err != nil {
		t.Fatalf("ipv6.Conn.SetTrafficClass failed: %v", err)
	}
	if v, err := c.TrafficClass(); err != nil {
		t.Fatalf("ipv6.Conn.TrafficClass failed: %v", err)
	} else if v != tclass {
		t.Fatalf("got unexpected traffic class %v; expected %v", v, tclass)
	}

	hoplim := 255
	if err := c.SetHopLimit(hoplim); err != nil {
		t.Fatalf("ipv6.Conn.SetHopLimit failed: %v", err)
	}
	if v, err := c.HopLimit(); err != nil {
		t.Fatalf("ipv6.Conn.HopLimit failed: %v", err)
	} else if v != hoplim {
		t.Fatalf("got unexpected hop limit %v; expected %v", v, hoplim)
	}
}
