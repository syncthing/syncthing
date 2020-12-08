// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.14,!noquic,!go1.16

package connections

import (
	"crypto/tls"
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
	qcs := q.Session.ConnectionState()
	return tls.ConnectionState{
		Version:                     qcs.Version,
		HandshakeComplete:           qcs.HandshakeComplete,
		DidResume:                   qcs.DidResume,
		CipherSuite:                 qcs.CipherSuite,
		NegotiatedProtocol:          qcs.NegotiatedProtocol,
		NegotiatedProtocolIsMutual:  qcs.NegotiatedProtocolIsMutual,
		ServerName:                  qcs.ServerName,
		PeerCertificates:            qcs.PeerCertificates,
		VerifiedChains:              qcs.VerifiedChains,
		SignedCertificateTimestamps: qcs.SignedCertificateTimestamps,
		OCSPResponse:                qcs.OCSPResponse,
		TLSUnique:                   qcs.TLSUnique,
	}
}

// Sort available packet connections by ip address, preferring unspecified local address.
func packetConnLess(i interface{}, j interface{}) bool {
	return util.AddressUnspecifiedLess(i.(net.PacketConn).LocalAddr(), j.(net.PacketConn).LocalAddr())
}
