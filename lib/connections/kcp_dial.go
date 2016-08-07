// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net/url"
	"time"

	"github.com/AudriusButkevicius/kcp-go"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

const kcpPriority = 50

func init() {
	factory := &kcpDialerFactory{}
	for _, scheme := range []string{"kcp", "kcp4", "kcp6"} {
		dialers[scheme] = factory
	}
}

type kcpDialer struct {
	cfg    *config.Wrapper
	tlsCfg *tls.Config
}

func (d *kcpDialer) Dial(id protocol.DeviceID, uri *url.URL) (IntermediateConnection, error) {
	uri = fixupPort(uri, 22020)

	conn, err := kcp.Dial(uri.Host)
	if err != nil {
		l.Debugln(err)
		return IntermediateConnection{}, err
	}

	conn.SetWindowSize(128, 128)
	conn.SetNoDelay(1, 10, 2, 1)

	tc := tls.Client(conn, d.tlsCfg)
	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return IntermediateConnection{}, err
	}

	return IntermediateConnection{tc, "KCP (Client)", kcpPriority}, nil
}

func (d *kcpDialer) RedialFrequency() time.Duration {
	return time.Duration(d.cfg.Options().ReconnectIntervalS) * time.Second
}

type kcpDialerFactory struct{}

func (kcpDialerFactory) New(cfg *config.Wrapper, tlsCfg *tls.Config) genericDialer {
	return &kcpDialer{
		cfg:    cfg,
		tlsCfg: tlsCfg,
	}
}

func (kcpDialerFactory) Priority() int {
	return kcpPriority
}

func (kcpDialerFactory) Enabled(cfg config.Configuration) bool {
	return true
}

func (kcpDialerFactory) String() string {
	return "KCP Dialer"
}
