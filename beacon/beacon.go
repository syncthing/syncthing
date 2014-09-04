// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package beacon

import "net"

type recv struct {
	data []byte
	src  net.Addr
}

type Interface interface {
	Send(data []byte)
	Recv() ([]byte, net.Addr)
}

func genericReader(conn *net.UDPConn, outbox chan<- recv) {
	bs := make([]byte, 65536)
	for {
		n, addr, err := conn.ReadFrom(bs)
		if err != nil {
			l.Warnln("multicast read:", err)
			return
		}
		if debug {
			l.Debugf("recv %d bytes from %s", n, addr)
		}

		c := make([]byte, n)
		copy(c, bs)
		select {
		case outbox <- recv{c, addr}:
		default:
			if debug {
				l.Debugln("dropping message")
			}
		}
	}
}
