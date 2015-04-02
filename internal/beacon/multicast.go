// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package beacon

import "net"

type Multicast struct {
	conn   *net.UDPConn
	addr   *net.UDPAddr
	intf   *net.Interface
	inbox  chan []byte
	outbox chan recv
}

func NewMulticast(addr, ifname string) (*Multicast, error) {
	gaddr, err := net.ResolveUDPAddr("udp6", addr)
	if err != nil {
		return nil, err
	}
	intf, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenMulticastUDP("udp6", intf, gaddr)
	if err != nil {
		return nil, err
	}
	b := &Multicast{
		conn:   conn,
		addr:   gaddr,
		intf:   intf,
		inbox:  make(chan []byte),
		outbox: make(chan recv, 16),
	}

	go genericReader(b.conn, b.outbox)
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
	addr := *b.addr
	addr.Zone = b.intf.Name
	for bs := range b.inbox {
		_, err := b.conn.WriteTo(bs, &addr)
		if err != nil && debug {
			l.Debugln(err, "on write to", addr)
		} else if debug {
			l.Debugf("sent %d bytes to %s", len(bs), addr.String())
		}
	}
}
