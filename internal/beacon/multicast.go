// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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
			l.Warnln("multicast interfaces:", err)
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
