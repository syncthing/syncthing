// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
