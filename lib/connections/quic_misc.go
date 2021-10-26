// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build go1.15 && !noquic
// +build go1.15,!noquic

package connections

import (
	"crypto/tls"
	"net"
	"net/url"

	"github.com/lucas-clemente/quic-go"
	"github.com/syncthing/syncthing/lib/util"
)

var (
	quicConfig = &quic.Config{
		ConnectionIDLength: 4,
		KeepAlive:          true,
	}
)

func quicNetwork(uri *url.URL) string {
	switch uri.Scheme {
	case "quic4":
		return "udp4"
	case "quic6":
		return "udp6"
	default:
		return "udp"
	}
}

type quicTlsConn struct {
	quic.Session
	quic.Stream
	// If we created this connection, we should be the ones closing it.
	createdConn net.PacketConn
}

func (q *quicTlsConn) Close() error {
	sterr := q.Stream.Close()
	seerr := q.Session.CloseWithError(0, "closing")
	var pcerr error
	if q.createdConn != nil {
		pcerr = q.createdConn.Close()
	}
	if sterr != nil {
		return sterr
	}
	if seerr != nil {
		return seerr
	}
	return pcerr
}

func (q *quicTlsConn) ConnectionState() tls.ConnectionState {
	return q.Session.ConnectionState().TLS.ConnectionState
}

// Sort available packet connections by ip address, preferring unspecified local address.
func packetConnLess(i interface{}, j interface{}) bool {
	return util.AddressUnspecifiedLess(i.(net.PacketConn).LocalAddr(), j.(net.PacketConn).LocalAddr())
}
