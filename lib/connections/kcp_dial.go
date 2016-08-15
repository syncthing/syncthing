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

	"github.com/hashicorp/yamux"

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

	// Try to dial via an existing listening connection
	// giving better changes punching through NAT.
	f := getDialingFilter()
	var conn *kcp.UDPSession
	var err error
	if f != nil {
		conn, err = kcp.Dial(uri.Host, kcpLogger, f.NewConn(20, &kcpConversationFilter{}))
		// We are piggy backing on a listener connection, no need for keepalives.
		// Futhermore, keepalives just send garbage, which will flip our filter.
		conn.SetKeepAlive(0)
		l.Debugf("dial %s using existing conn on %s", uri.String(), conn.LocalAddr())
	} else {
		conn, err = kcp.Dial(uri.Host, kcpLogger)
	}
	if err != nil {
		l.Debugln(err)
		conn.Close()
		return IntermediateConnection{}, err
	}

	conn.SetWindowSize(128, 128)
	conn.SetNoDelay(1, 10, 2, 1)

	ses, err := yamux.Client(conn, yamuxCfg)
	if err != nil {
		conn.Close()
		return IntermediateConnection{}, err
	}
	stream, err := ses.OpenStream()
	if err != nil {
		ses.Close()
		return IntermediateConnection{}, err
	}

	tc := tls.Client(&sessionClosingStream{stream}, d.tlsCfg)
	tc.SetDeadline(time.Now().Add(time.Second * 10))
	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return IntermediateConnection{}, err
	}
	tc.SetDeadline(time.Time{})

	return IntermediateConnection{tc, "KCP (Client)", kcpPriority}, nil
}

func (d *kcpDialer) RedialFrequency() time.Duration {
	// For restricted NATs, the mapping UDP will potentially only be open for 20-30 seconds
	// hence try dialing just as often.
	return time.Duration(d.cfg.Options().StunKeepaliveS) * time.Second
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
