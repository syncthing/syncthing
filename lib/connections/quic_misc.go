// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !noquic

package connections

import (
	"crypto/tls"
	"io"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
)

var quicConfig = &quic.Config{
	MaxIdleTimeout:  30 * time.Second,
	KeepAlivePeriod: 15 * time.Second,
}

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
	*quic.Conn
	*quic.Stream

	// Keeps the manager and its endpoint alive until this connection closes.
	lease io.Closer
}

func (q *quicTlsConn) Close() error {
	sterr := q.Stream.Close()
	seerr := q.Conn.CloseWithError(0, "closing")
	var pcerr error
	if q.lease != nil {
		pcerr = q.lease.Close()
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
	return q.Conn.ConnectionState().TLS
}
