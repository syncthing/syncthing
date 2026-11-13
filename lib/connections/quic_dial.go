// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build go1.15 && !noquic

package connections

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

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

	ctx, cancel := context.WithTimeout(ctx, quicOperationTimeout)
	defer cancel()
	session, err := d.registry.UDP.Dial(ctx, addr.AddrPort(), d.tlsCfg, quicConfig)
	if err != nil {
		return internalConn{}, fmt.Errorf("dial: %w", err)
	}
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		_ = session.CloseWithError(1, err.Error())
		return internalConn{}, fmt.Errorf("open stream: %w", err)
	}

	priority := d.wanPriority
	isLocal := d.lanChecker.isLAN(session.RemoteAddr())
	if isLocal {
		priority = d.lanPriority
	}

	return newInternalConn(&quicTlsConn{session, stream}, connTypeQUICClient, isLocal, priority), nil
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
