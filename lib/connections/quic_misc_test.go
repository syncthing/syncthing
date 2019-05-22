// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.12

package connections

import (
	"net"
	"sort"
	"testing"
	"time"
)

type mockPacketConn struct {
	addr net.Addr
}

func (mockPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	panic("implement me")
}

func (mockPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	panic("implement me")
}

func (mockPacketConn) Close() error {
	panic("implement me")
}

func (c *mockPacketConn) LocalAddr() net.Addr {
	return c.addr
}

func (mockPacketConn) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (mockPacketConn) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (mockPacketConn) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}

func TestPacketConnUnspecifiedVsSpecified(t *testing.T) {
	addr1, err := net.ResolveTCPAddr("tcp", "127.0.0.1:1234")
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := net.ResolveTCPAddr("tcp", ":1235")
	if err != nil {
		t.Fatal(err)
	}

	conns := []*mockPacketConn{
		{addr1},
		{addr2},
	}

	sort.Slice(conns, func(i, j int) bool {
		return packetConnLess(conns[i], conns[j])
	})

	if conns[0].addr != addr2 {
		t.Error("unexpected")
	}
}

func TestPacketConnUnspecifiedVsSpecifiedNonEmpty(t *testing.T) {
	addr1, err := net.ResolveTCPAddr("tcp", "127.0.0.1:1234")
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := net.ResolveTCPAddr("tcp", "0.0.0.0:1235")
	if err != nil {
		t.Fatal(err)
	}

	conns := []*mockPacketConn{
		{addr1},
		{addr2},
	}

	sort.Slice(conns, func(i, j int) bool {
		return packetConnLess(conns[i], conns[j])
	})

	if conns[0].addr != addr2 {
		t.Error("unexpected")
	}
}

func TestPacketConnTCPvsTCP4(t *testing.T) {
	addr1, err := net.ResolveTCPAddr("tcp4", "0.0.0.0:1234")
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := net.ResolveTCPAddr("tcp", "0.0.0.0:1235")
	if err != nil {
		t.Fatal(err)
	}

	conns := []*mockPacketConn{
		{addr1},
		{addr2},
	}

	sort.Slice(conns, func(i, j int) bool {
		return packetConnLess(conns[i], conns[j])
	})

	if conns[0].addr != addr2 {
		t.Error("unexpected")
	}
}
