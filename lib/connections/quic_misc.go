// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !noquic
// +build !noquic

package connections

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"

	"github.com/syncthing/syncthing/lib/osutil"
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
	quic.Connection
	quic.Stream
	// If we created this connection, we should be the ones closing it.
	createdConn net.PacketConn
}

func (q *quicTlsConn) Close() error {
	sterr := q.Stream.Close()
	seerr := q.Connection.CloseWithError(0, "closing")
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
	return q.Connection.ConnectionState().TLS
}

func transportConnUnspecified(conn any) bool {
	tran, ok := conn.(*quic.Transport)
	if !ok {
		return false
	}
	addr := tran.Conn.LocalAddr()
	ip, err := osutil.IPFromAddr(addr)
	return err == nil && ip.IsUnspecified()
}

type writeTrackingTracer struct {
	lastWrite atomic.Int64 // unix nanos
}

func (t *writeTrackingTracer) loggingTracer() *logging.Tracer {
	return &logging.Tracer{
		SentPacket: func(net.Addr, *logging.Header, logging.ByteCount, []logging.Frame) {
			t.lastWrite.Store(time.Now().UnixNano())
		},
		SentVersionNegotiationPacket: func(net.Addr, logging.ArbitraryLenConnectionID, logging.ArbitraryLenConnectionID, []logging.Version) {
			t.lastWrite.Store(time.Now().UnixNano())
		},
	}
}

func (t *writeTrackingTracer) LastWrite() time.Time {
	return time.Unix(0, t.lastWrite.Load())
}

// A transportPacketConn is a net.PacketConn that uses a quic.Transport.
type transportPacketConn struct {
	tran         *quic.Transport
	readDeadline atomic.Value // time.Time
}

func (t *transportPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	ctx := context.Background()
	if deadline, ok := t.readDeadline.Load().(time.Time); ok && !deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	return t.tran.ReadNonQUICPacket(ctx, p)
}

func (t *transportPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return t.tran.WriteTo(p, addr)
}

func (*transportPacketConn) Close() error {
	return errUnsupported
}

func (t *transportPacketConn) LocalAddr() net.Addr {
	return t.tran.Conn.LocalAddr()
}

func (t *transportPacketConn) SetDeadline(deadline time.Time) error {
	return t.SetReadDeadline(deadline)
}

func (t *transportPacketConn) SetReadDeadline(deadline time.Time) error {
	t.readDeadline.Store(deadline)
	return nil
}

func (*transportPacketConn) SetWriteDeadline(_ time.Time) error {
	return nil // yolo
}
