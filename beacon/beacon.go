// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package beacon

import "net"

type recv struct {
	data []byte
	src  net.Addr
}

type dst struct {
	intf string
	conn *net.UDPConn
}

type Beacon struct {
	conn   *net.UDPConn
	port   int
	conns  []dst
	inbox  chan []byte
	outbox chan recv
}

func New(port int) (*Beacon, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return nil, err
	}
	b := &Beacon{
		conn:   conn,
		port:   port,
		inbox:  make(chan []byte),
		outbox: make(chan recv, 16),
	}

	go b.reader()
	go b.writer()

	return b, nil
}

func (b *Beacon) Send(data []byte) {
	b.inbox <- data
}

func (b *Beacon) Recv() ([]byte, net.Addr) {
	recv := <-b.outbox
	return recv.data, recv.src
}

func (b *Beacon) reader() {
	bs := make([]byte, 65536)
	for {
		n, addr, err := b.conn.ReadFrom(bs)
		if err != nil {
			l.Warnln("Beacon read:", err)
			return
		}
		if debug {
			l.Debugf("recv %d bytes from %s", n, addr)
		}

		c := make([]byte, n)
		copy(c, bs)
		select {
		case b.outbox <- recv{c, addr}:
		default:
			if debug {
				l.Debugln("dropping message")
			}
		}
	}
}

func (b *Beacon) writer() {
	for bs := range b.inbox {

		addrs, err := net.InterfaceAddrs()
		if err != nil {
			l.Warnln("Beacon: interface addresses:", err)
			continue
		}

		var dsts []net.IP
		for _, addr := range addrs {
			if iaddr, ok := addr.(*net.IPNet); ok && iaddr.IP.IsGlobalUnicast() && iaddr.IP.To4() != nil {
				baddr := bcast(iaddr)
				dsts = append(dsts, baddr.IP)
			}
		}

		if len(dsts) == 0 {
			// Fall back to the general IPv4 broadcast address
			dsts = append(dsts, net.IP{0xff, 0xff, 0xff, 0xff})
		}

		if debug {
			l.Debugln("addresses:", dsts)
		}

		for _, ip := range dsts {
			dst := &net.UDPAddr{IP: ip, Port: b.port}

			_, err := b.conn.WriteTo(bs, dst)
			if err != nil {
				if debug {
					l.Debugln(err)
				}
			} else if debug {
				l.Debugf("sent %d bytes to %s", len(bs), dst)
			}
		}
	}
}

func bcast(ip *net.IPNet) *net.IPNet {
	var bc = &net.IPNet{}
	bc.IP = make([]byte, len(ip.IP))
	copy(bc.IP, ip.IP)
	bc.Mask = ip.Mask

	offset := len(bc.IP) - len(bc.Mask)
	for i := range bc.IP {
		if i-offset >= 0 {
			bc.IP[i] = ip.IP[i] | ^ip.Mask[i-offset]
		}
	}
	return bc
}
