// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package registry

import (
	"errors"
	"net"
	"syscall"

	"golang.org/x/net/ipv4"
)

type batchReader interface {
	ReadBatch([]ipv4.Message, int) (int, error)
}

type udpConn struct {
	*net.UDPConn

	batch batchReader
}

func listenUDP(network string, addr *net.UDPAddr) (net.PacketConn, error) {
	conn, err := net.ListenUDP(network, addr)
	if err != nil {
		return nil, err
	}
	return &udpConn{UDPConn: conn, batch: ipv4.NewPacketConn(conn)}, nil
}

func (c *udpConn) ReadBatch(messages []ipv4.Message, flags int) (int, error) {
	// hide some errors that would otherwise cause the socket to close
	// ts=2026-11-05T17:40:37.213+01:00 level=error source=github.com/libp2p/go-libp2p@v0.50.0/p2p/net/swarm/swarm_listen.go:151 msg="swarm listener unintentionally closed" logger=swarm2
	for {
		n, err := c.batch.ReadBatch(messages, flags)
		if !isUDPDestinationError(err) {
			return n, err
		}
		if n > 0 {
			return n, nil
		}
	}
}

func (c *udpConn) ReadMsgUDP(b, oob []byte) (int, int, int, *net.UDPAddr, error) {
	for {
		n, on, flags, addr, err := c.UDPConn.ReadMsgUDP(b, oob)
		if !isUDPDestinationError(err) {
			return n, on, flags, addr, err
		}
	}
}

func (c *udpConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.UDPConn.ReadFrom(b)
		if !isUDPDestinationError(err) {
			return n, addr, err
		}
	}
}

func isUDPDestinationError(err error) bool {
	return errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTDOWN) || errors.Is(err, syscall.ENETDOWN) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET)
}
