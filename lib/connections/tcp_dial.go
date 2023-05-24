// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"crypto/tls"
	"math/rand"
	"net/url"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/protocol"
)

func init() {
	factory := &tcpDialerFactory{}
	for _, scheme := range []string{"tcp", "tcp4", "tcp6"} {
		dialers[scheme] = factory
	}
}

type tcpDialer struct {
	commonDialer
	registry *registry.Registry
}

func (d *tcpDialer) Dial(ctx context.Context, _ protocol.DeviceID, uri *url.URL) (internalConn, error) {
	uri = fixupPort(uri, config.DefaultTCPPort)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := dialer.DialContextReusePortFunc(d.registry)(timeoutCtx, uri.Scheme, uri.Host)
	if err != nil {
		return internalConn{}, err
	}

	err = dialer.SetTCPOptions(conn)
	if err != nil {
		l.Debugln("Dial (BEP/tcp): setting tcp options:", err)
	}

	err = dialer.SetTrafficClass(conn, d.trafficClass)
	if err != nil {
		l.Debugln("Dial (BEP/tcp): setting traffic class:", err)
	}

	tc := tls.Client(conn, d.tlsCfg)
	err = tlsTimedHandshake(tc)
	if err != nil {
		tc.Close()
		return internalConn{}, err
	}

	priority := d.wanPriority
	isLocal := d.lanChecker.isLAN(conn.RemoteAddr())
	if isLocal {
		priority = d.lanPriority
	}

	// XXX: Induced flakyness
	if dur, _ := time.ParseDuration(os.Getenv("CONN_FLAKY_LIFETIME")); dur > 0 {
		dur = dur/2 + time.Duration(rand.Intn(int(dur)))
		time.AfterFunc(dur, func() {
			tc.Close()
		})
	}

	return newInternalConn(tc, connTypeTCPClient, isLocal, priority), nil
}

type tcpDialerFactory struct{}

func (tcpDialerFactory) New(opts config.OptionsConfiguration, tlsCfg *tls.Config, registry *registry.Registry, lanChecker *lanChecker) genericDialer {
	return &tcpDialer{
		commonDialer: commonDialer{
			trafficClass:      opts.TrafficClass,
			reconnectInterval: time.Duration(opts.ReconnectIntervalS) * time.Second,
			tlsCfg:            tlsCfg,
			lanPriority:       opts.ConnectionPriorityTCPLAN,
			wanPriority:       opts.ConnectionPriorityTCPWAN,
			lanChecker:        lanChecker,
		},
		registry: registry,
	}
}

func (tcpDialerFactory) AlwaysWAN() bool {
	return false
}

func (tcpDialerFactory) Valid(_ config.Configuration) error {
	// Always valid
	return nil
}

func (tcpDialerFactory) String() string {
	return "TCP Dialer"
}
