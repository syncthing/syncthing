// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build go1.12

package connections

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"time"

	"github.com/lucas-clemente/quic-go"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/protocol"
)

const quicPriority = 100

func init() {
	factory := &quicDialerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		dialers[scheme] = factory
	}
}

type quicDialer struct {
	cfg    config.Wrapper
	tlsCfg *tls.Config
}

func (d *quicDialer) Dial(id protocol.DeviceID, uri *url.URL) (internalConn, error) {
	uri = fixupPort(uri, config.DefaultQUICPort)

	addr, err := net.ResolveUDPAddr("udp", uri.Host)
	if err != nil {
		return internalConn{}, err
	}

	var conn net.PacketConn
	closeConn := false
	if listenConn := registry.Get(uri.Scheme, packetConnLess); listenConn != nil {
		conn = listenConn.(net.PacketConn)
	} else {
		if packetConn, err := net.ListenPacket("udp", ":0"); err != nil {
			return internalConn{}, err
		} else {
			closeConn = true
			conn = packetConn
		}
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	session, err := quic.DialContext(ctx, conn, addr, uri.Host, d.tlsCfg, quicConfig)
	if err != nil {
		if closeConn {
			_ = conn.Close()
		}
		return internalConn{}, err
	}

	// OpenStreamSync is blocks, but we want to make sure the connection is usable
	// before we start killing off other connections, so do the dance.
	ok := make(chan struct{})
	go func() {
		select {
		case <-ok:
			return
		case <-time.After(10 * time.Second):
			l.Debugln("timed out waiting for OpenStream on", session.RemoteAddr())
			// This will unblock OpenStreamSync
			_ = session.Close()
		}
	}()

	stream, err := session.OpenStreamSync()
	close(ok)
	if err != nil {
		// It's ok to close these, this does not close the underlying packetConn.
		_ = session.Close()
		if closeConn {
			_ = conn.Close()
		}
		return internalConn{}, err
	}

	return internalConn{&quicTlsConn{session, stream}, connTypeQUICClient, quicPriority}, nil
}

func (d *quicDialer) RedialFrequency() time.Duration {
	return time.Duration(d.cfg.Options().ReconnectIntervalS) * time.Second
}

type quicDialerFactory struct {
	cfg    config.Wrapper
	tlsCfg *tls.Config
}

func (quicDialerFactory) New(cfg config.Wrapper, tlsCfg *tls.Config) genericDialer {
	return &quicDialer{
		cfg:    cfg,
		tlsCfg: tlsCfg,
	}
}

func (quicDialerFactory) Priority() int {
	return quicPriority
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
