// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.12

package connections

import (
	"net"

	"github.com/lucas-clemente/quic-go"
	"github.com/syncthing/syncthing/lib/util"
)

var (
	quicConfig = &quic.Config{
		ConnectionIDLength: 4,
		KeepAlive:          true,
	}
)

type quicTlsConn struct {
	quic.Session
	quic.Stream
}

func (q *quicTlsConn) Close() error {
	sterr := q.Stream.Close()
	seerr := q.Session.Close()
	if sterr != nil {
		return sterr
	}
	return seerr
}

// Sort available packet connections by ip address, preferring unspecified local address.
func packetConnLess(i interface{}, j interface{}) bool {
	return util.AddressUnspecifiedLess(i.(net.PacketConn).LocalAddr(), j.(net.PacketConn).LocalAddr())
}
