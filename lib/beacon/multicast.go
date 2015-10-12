// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package beacon

import (
	"errors"
	"net"

	"golang.org/x/net/ipv6"
)

type Multicast struct {
	conn   *ipv6.PacketConn
	addr   *net.UDPAddr
	inbox  chan []byte
	outbox chan recv
	intfs  []net.Interface
}

func NewMulticast(addr string) (*Multicast, error) {
	gaddr, err := net.ResolveUDPAddr("udp6", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenPacket("udp6", addr)
	if err != nil {
		return nil, err
	}

	intfs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	p := ipv6.NewPacketConn(conn)
	joined := 0
	for _, intf := range intfs {
		err := p.JoinGroup(&intf, &net.UDPAddr{IP: gaddr.IP})
		if debug {
			if err != nil {
				l.Debugln("IPv6 join", intf.Name, "failed:", err)
			} else {
				l.Debugln("IPv6 join", intf.Name, "success")
			}
		}
		joined++
	}

	if joined == 0 {
		return nil, errors.New("no multicast interfaces available")
	}

	b := &Multicast{
		conn:   p,
		addr:   gaddr,
		inbox:  make(chan []byte),
		outbox: make(chan recv, 16),
		intfs:  intfs,
	}

	go genericReader(ipv6ReaderAdapter{b.conn}, b.outbox)
	go b.writer()

	return b, nil
}

func (b *Multicast) Send(data []byte) {
	b.inbox <- data
}

func (b *Multicast) Recv() ([]byte, net.Addr) {
	recv := <-b.outbox
	return recv.data, recv.src
}

func (b *Multicast) writer() {
	wcm := &ipv6.ControlMessage{
		HopLimit: 1,
	}

	for bs := range b.inbox {
		for _, intf := range b.intfs {
			wcm.IfIndex = intf.Index
			_, err := b.conn.WriteTo(bs, wcm, b.addr)
			if err != nil && debug {
				l.Debugln(err, "on write to", b.addr)
			} else if debug {
				l.Debugf("sent %d bytes to %v on %s", len(bs), b.addr, intf.Name)
			}
		}
	}
}

// This makes ReadFrom on an *ipv6.PacketConn behave like ReadFrom on a
// net.PacketConn.
type ipv6ReaderAdapter struct {
	c *ipv6.PacketConn
}

func (i ipv6ReaderAdapter) ReadFrom(bs []byte) (int, net.Addr, error) {
	n, _, src, err := i.c.ReadFrom(bs)
	return n, src, err
}
