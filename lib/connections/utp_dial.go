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

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/utp"
)

const utpPriority = 50

func init() {
	factory := &utpDialerFactory{}
	for _, scheme := range []string{"utp", "utp4", "utp6"} {
		dialers[scheme] = factory
	}
}

type utpDialer struct {
	cfg    *config.Wrapper
	tlsCfg *tls.Config
}

func (d *utpDialer) Dial(id protocol.DeviceID, uri *url.URL) (IntermediateConnection, error) {
	uri = fixupPort(uri, 22020)

	conn, err := utp.DialTimeout(uri.Host, 10*time.Second)
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

	return IntermediateConnection{tc, "UTP (Client)", utpPriority}, nil
}

func (d *utpDialer) RedialFrequency() time.Duration {
	return time.Duration(d.cfg.Options().ReconnectIntervalS) * time.Second
}

type utpDialerFactory struct{}

func (utpDialerFactory) New(cfg *config.Wrapper, tlsCfg *tls.Config) genericDialer {
	return &utpDialer{
		cfg:    cfg,
		tlsCfg: tlsCfg,
	}
}

func (utpDialerFactory) Priority() int {
	return utpPriority
}

func (utpDialerFactory) Enabled(cfg config.Configuration) bool {
	return true
}

func (utpDialerFactory) String() string {
	return "UTP Dialer"
}
