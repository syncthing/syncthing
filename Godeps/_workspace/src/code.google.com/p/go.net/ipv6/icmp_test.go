// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6_test

import (
	"code.google.com/p/go.net/ipv6"
	"net"
	"os"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

func TestICMPFilter(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}

	var f ipv6.ICMPFilter
	for _, toggle := range []bool{false, true} {
		f.SetAll(toggle)
		var wg sync.WaitGroup
		for _, typ := range []ipv6.ICMPType{
			ipv6.ICMPTypeDestinationUnreachable,
			ipv6.ICMPTypeEchoReply,
			ipv6.ICMPTypeNeighborSolicitation,
			ipv6.ICMPTypeDuplicateAddressConfirmation,
		} {
			wg.Add(1)
			go func(typ ipv6.ICMPType) {
				defer wg.Done()
				f.Set(typ, false)
				if f.WillBlock(typ) {
					t.Errorf("ipv6.ICMPFilter.Set(%v, false) failed", typ)
				}
				f.Set(typ, true)
				if !f.WillBlock(typ) {
					t.Errorf("ipv6.ICMPFilter.Set(%v, true) failed", typ)
				}
			}(typ)
		}
		wg.Wait()
	}
}

func TestSetICMPFilter(t *testing.T) {
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

	p := ipv6.NewPacketConn(c)

	var f ipv6.ICMPFilter
	f.SetAll(true)
	f.Set(ipv6.ICMPTypeEchoRequest, false)
	f.Set(ipv6.ICMPTypeEchoReply, false)
	if err := p.SetICMPFilter(&f); err != nil {
		t.Fatalf("ipv6.PacketConn.SetICMPFilter failed: %v", err)
	}
	kf, err := p.ICMPFilter()
	if err != nil {
		t.Fatalf("ipv6.PacketConn.ICMPFilter failed: %v", err)
	}
	if !reflect.DeepEqual(kf, &f) {
		t.Fatalf("got unexpected filter %#v; expected %#v", kf, f)
	}
}
