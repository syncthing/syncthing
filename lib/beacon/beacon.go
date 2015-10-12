// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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

type readerFrom interface {
	ReadFrom([]byte) (int, net.Addr, error)
}

func genericReader(conn readerFrom, outbox chan<- recv) {
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
