// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.12

package connections

import (
	"net"
	"testing"
	"time"
)

type mockPacketConn struct {
	addr mockedAddr
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

type mockedAddr struct {
	network string
	addr    string
}

func (a mockedAddr) Network() string {
	return a.network
}

func (a mockedAddr) String() string {
	return a.addr
}

func TestPacketConnLess(t *testing.T) {
	cases := []struct {
		netA  string
		addrA string
		netB  string
		addrB string
	}{
		// B is assumed the winner.
		{"tcp", "127.0.0.1:1234", "tcp", ":1235"},
		{"tcp", "127.0.0.1:1234", "tcp", "0.0.0.0:1235"},
		{"tcp4", "0.0.0.0:1234", "tcp", "0.0.0.0:1235"}, // tcp4 on the first one
	}

	for i, testCase := range cases {

		conns := []*mockPacketConn{
			{mockedAddr{testCase.netA, testCase.addrA}},
			{mockedAddr{testCase.netB, testCase.addrB}},
		}

		if packetConnLess(conns[0], conns[1]) {
			t.Error(i, "unexpected")
		}
		if !packetConnLess(conns[1], conns[0]) {
			t.Error(i, "unexpected")
		}
		if packetConnLess(conns[0], conns[0]) || packetConnLess(conns[1], conns[1]) {
			t.Error(i, "unexpected")
		}
	}
}
