// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay/client"
)

func init() {
	dialers["relay"] = relayDialerFactory{}
}

type relayDialer struct {
	cfg    *config.Wrapper
	tlsCfg *tls.Config
}

func (d *relayDialer) Dial(id protocol.DeviceID, uri *url.URL) (internalConn, error) {
	inv, err := client.GetInvitationFromRelay(uri, id, d.tlsCfg.Certificates, 10*time.Second)
	if err != nil {
		return internalConn{}, err
	}

	conn, err := client.JoinSession(inv)
	if err != nil {
		return internalConn{}, err
	}

	err = dialer.SetTCPOptions(conn)
	if err != nil {
		conn.Close()
		return internalConn{}, err
	}

	err = dialer.SetTrafficClass(conn, d.cfg.Options().TrafficClass)
	if err != nil {
		l.Debugln("Dial (BEP/relay): setting traffic class:", err)
	}

	var tc *tls.Conn
	if inv.ServerSocket {
		tc = tls.Server(conn, d.tlsCfg)
	} else {
		tc = tls.Client(conn, d.tlsCfg)
	}

	err = tlsTimedHandshake(tc)
	if err != nil {
		tc.Close()
		return internalConn{}, err
	}

	return internalConn{tc, connTypeRelayClient, relayPriority}, nil
}

func (d *relayDialer) RedialFrequency() time.Duration {
	return time.Duration(d.cfg.Options().RelayReconnectIntervalM) * time.Minute
}

type relayDialerFactory struct{}

func (relayDialerFactory) New(cfg *config.Wrapper, tlsCfg *tls.Config) genericDialer {
	return &relayDialer{
		cfg:    cfg,
		tlsCfg: tlsCfg,
	}
}

func (relayDialerFactory) Priority() int {
	return relayPriority
}

func (relayDialerFactory) AlwaysWAN() bool {
	return true
}

func (relayDialerFactory) Enabled(cfg config.Configuration) bool {
	return cfg.Options.RelaysEnabled
}

func (relayDialerFactory) String() string {
	return "Relay Dialer"
}
