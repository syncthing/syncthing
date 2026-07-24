// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build go1.15 && !noquic
// +build go1.15,!noquic

package connections

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	// The timeout for connecting, accepting and creating the various
	// streams.
	quicOperationTimeout = 10 * time.Second
)

func init() {
	factory := &quicDialerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		dialers[scheme] = factory
	}
}

type quicDialer struct {
	commonDialer

	registry *registry.Registry
}

func (d *quicDialer) Dial(ctx context.Context, _ protocol.DeviceID, uri *url.URL) (internalConn, error) {
	uri = fixupPort(uri, config.DefaultQUICPort)

	network := quicNetwork(uri)

	addr, err := net.ResolveUDPAddr(network, uri.Host)
	if err != nil {
		return internalConn{}, err
	}

	transport, _ := d.registry.Get(uri.Scheme, transportConnUnspecified).(*quic.Transport)
	if transport != nil {
		conn, err := d.dial(ctx, transport, nil, addr)
		if err == nil {
			return conn, nil
		}
		if !isQUICDialTimeout(err) {
			return internalConn{}, err
		}

		// The shared listener transport is useful for NAT traversal, but can
		// get wedged in quic-go cleanup after network sleep/wake. Retry once
		// with a fresh socket so the connection loop is not tied to that state.
		l.Debugf("Dial (BEP/quic): shared transport timed out for %s, retrying with fresh transport: %v", uri, err)
	}

	return d.dialWithFreshTransport(ctx, addr)
}

type quicDialResult struct {
	conn internalConn
	err  error
}

func (d *quicDialer) dialWithFreshTransport(ctx context.Context, addr *net.UDPAddr) (internalConn, error) {
	packetConn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return internalConn{}, err
	}
	return d.dial(ctx, &quic.Transport{Conn: packetConn}, packetConn, addr)
}

func (d *quicDialer) dial(ctx context.Context, transport *quic.Transport, createdConn net.PacketConn, addr *net.UDPAddr) (internalConn, error) {
	ctx, cancel := context.WithTimeout(ctx, quicOperationTimeout)
	defer cancel()

	res := make(chan quicDialResult)
	go func() {
		conn, err := d.dialAndOpenStream(ctx, transport, createdConn, addr)
		result := quicDialResult{conn: conn, err: err}
		select {
		case res <- result:
		case <-ctx.Done():
			if err == nil {
				_ = conn.Close()
			}
		}
	}()

	select {
	case result := <-res:
		return result.conn, result.err
	case <-ctx.Done():
		if createdConn != nil {
			_ = createdConn.Close()
		}
		return internalConn{}, ctx.Err()
	}
}

func (d *quicDialer) dialAndOpenStream(ctx context.Context, transport *quic.Transport, createdConn net.PacketConn, addr *net.UDPAddr) (internalConn, error) {
	session, err := transport.Dial(ctx, addr, d.tlsCfg, quicConfig)
	if err != nil {
		if createdConn != nil {
			_ = createdConn.Close()
		}
		return internalConn{}, fmt.Errorf("dial: %w", err)
	}

	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		// Close asynchronously: quic-go waits for connection teardown here,
		// and a wedged teardown must not block the dial result.
		go session.CloseWithError(1, err.Error())
		if createdConn != nil {
			_ = createdConn.Close()
		}
		return internalConn{}, fmt.Errorf("open stream: %w", err)
	}

	priority := d.wanPriority
	isLocal := d.lanChecker.isLAN(session.RemoteAddr())
	if isLocal {
		priority = d.lanPriority
	}

	return newInternalConn(&quicTlsConn{session, stream, createdConn}, connTypeQUICClient, isLocal, priority), nil
}

func isQUICDialTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "no recent network activity")
}

type quicDialerFactory struct{}

func (quicDialerFactory) New(opts config.OptionsConfiguration, tlsCfg *tls.Config, registry *registry.Registry, lanChecker *lanChecker) genericDialer {
	return &quicDialer{
		commonDialer: commonDialer{
			reconnectInterval: time.Duration(opts.ReconnectIntervalS) * time.Second,
			tlsCfg:            tlsCfg,
			lanChecker:        lanChecker,
			lanPriority:       opts.ConnectionPriorityQUICLAN,
			wanPriority:       opts.ConnectionPriorityQUICWAN,
			allowsMultiConns:  true,
		},
		registry: registry,
	}
}

func (quicDialerFactory) AlwaysWAN() bool {
	return false
}

func (quicDialerFactory) Valid(_ config.Configuration) error {
	// Always valid
	return nil
}

func (quicDialerFactory) String() string {
	return "QUIC Dialer"
}
