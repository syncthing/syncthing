// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/protocol"
)

const tcpPriority = 10

func init() {
	for _, scheme := range []string{"tcp", "tcp4", "tcp6"} {
		dialers[scheme] = tcpDialerFactory{}
	}
}

type tcpDialer struct {
	cfg    *config.Wrapper
	tlsCfg *tls.Config
}

func (d *tcpDialer) Dial(id protocol.DeviceID, uri *url.URL) (IntermediateConnection, error) {
	uri = fixupPort(uri)

	raddr, err := net.ResolveTCPAddr(uri.Scheme, uri.Host)
	if err != nil {
		l.Debugln(err)
		return IntermediateConnection{}, err
	}

	conn, err := dialer.DialTimeout(raddr.Network(), raddr.String(), 10*time.Second)
	if err != nil {
		l.Debugln(err)
		return IntermediateConnection{}, err
	}

	tc := tls.Client(conn, d.tlsCfg)
	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return IntermediateConnection{}, err
	}

	return IntermediateConnection{tc, "TCP (Client)", tcpPriority}, nil
}

func (tcpDialer) Priority() int {
	return tcpPriority
}

func (d *tcpDialer) RedialFrequency() time.Duration {
	return time.Duration(d.cfg.Options().ReconnectIntervalS) * time.Second
}

func (d *tcpDialer) String() string {
	return "TCP Dialer"
}

type tcpDialerFactory struct{}

func (tcpDialerFactory) New(cfg *config.Wrapper, tlsCfg *tls.Config) genericDialer {
	return &tcpDialer{
		cfg:    cfg,
		tlsCfg: tlsCfg,
	}
}

func (tcpDialerFactory) Priority() int {
	return tcpPriority
}
