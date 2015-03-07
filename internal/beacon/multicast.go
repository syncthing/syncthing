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
	inbox  chan []byte
	outbox chan recv
}

func NewMulticast(addr string) (*Multicast, error) {
	gaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenMulticastUDP("udp", nil, gaddr)
	if err != nil {
		return nil, err
	}
	b := &Multicast{
		conn:   conn,
		addr:   gaddr,
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
	for bs := range b.inbox {
		intfs, err := net.Interfaces()
		if err != nil {
			if debug {
				l.Debugln("multicast interfaces:", err)
			}
			continue
		}
		for _, intf := range intfs {
			if intf.Flags&net.FlagUp != 0 && intf.Flags&net.FlagMulticast != 0 {
				addr := *b.addr
				addr.Zone = intf.Name
				_, err = b.conn.WriteTo(bs, &addr)
				if err != nil {
					if debug {
						l.Debugln(err, "on write to", addr)
					}
				} else if debug {
					l.Debugf("sent %d bytes to %s", len(bs), addr.String())
				}
			}
		}
	}
}
