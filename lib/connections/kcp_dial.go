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
	"github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"
)

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

func (d *kcpDialer) Dial(id protocol.DeviceID, uri *url.URL) (internalConn, error) {
	uri = fixupPort(uri, config.DefaultKCPPort)

	var conn *kcp.UDPSession
	var err error

	// Try to dial via an existing listening connection
	// giving better changes punching through NAT.
	if f := getDialingFilter(); f != nil {
		conn, err = kcp.NewConn(uri.Host, nil, 0, 0, f.NewConn(kcpConversationFilterPriority, &kcpConversationFilter{}))
		l.Debugf("dial %s using existing conn on %s", uri.String(), conn.LocalAddr())
	} else {
		conn, err = kcp.DialWithOptions(uri.Host, nil, 0, 0)
	}
	if err != nil {
		return internalConn{}, err
	}

	opts := d.cfg.Options()

	conn.SetStreamMode(true)
	conn.SetACKNoDelay(false)
	conn.SetWindowSize(opts.KCPSendWindowSize, opts.KCPReceiveWindowSize)
	conn.SetNoDelay(boolInt(opts.KCPNoDelay), opts.KCPUpdateIntervalMs, boolInt(opts.KCPFastResend), boolInt(!opts.KCPCongestionControl))

	ses, err := smux.Client(conn, smuxConfig)
	if err != nil {
		conn.Close()
		return internalConn{}, err
	}

	ses.SetDeadline(time.Now().Add(10 * time.Second))
	stream, err := ses.OpenStream()
	if err != nil {
		ses.Close()
		return internalConn{}, err
	}
	ses.SetDeadline(time.Time{})

	tc := tls.Client(&sessionClosingStream{stream, ses}, d.tlsCfg)
	tc.SetDeadline(time.Now().Add(time.Second * 10))
	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return internalConn{}, err
	}
	tc.SetDeadline(time.Time{})

	return internalConn{tc, connTypeKCPClient, kcpPriority}, nil
}

func (d *kcpDialer) RedialFrequency() time.Duration {
	// For restricted NATs, the UDP mapping will potentially only be open for 20-30 seconds
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

func (kcpDialerFactory) AlwaysWAN() bool {
	return false
}

func (kcpDialerFactory) Enabled(cfg config.Configuration) bool {
	return true
}

func (kcpDialerFactory) String() string {
	return "KCP Dialer"
}
