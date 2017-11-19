// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
)

func init() {
	factory := &tcpListenerFactory{}
	for _, scheme := range []string{"tcp", "tcp4", "tcp6"} {
		listeners[scheme] = factory
	}
}

type tcpListener struct {
	onAddressesChangedNotifier

	uri     *url.URL
	cfg     *config.Wrapper
	tlsCfg  *tls.Config
	stop    chan struct{}
	conns   chan internalConn
	factory listenerFactory

	natService *nat.Service
	mapping    *nat.Mapping

	err error
	mut sync.RWMutex
}

func (t *tcpListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	tcaddr, err := net.ResolveTCPAddr(t.uri.Scheme, t.uri.Host)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("Listen (BEP/tcp):", err)
		return
	}

	listener, err := net.ListenTCP(t.uri.Scheme, tcaddr)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("Listen (BEP/tcp):", err)
		return
	}
	defer listener.Close()

	l.Infof("TCP listener (%v) starting", listener.Addr())
	defer l.Infof("TCP listener (%v) shutting down", listener.Addr())

	mapping := t.natService.NewMapping(nat.TCP, tcaddr.IP, tcaddr.Port)
	mapping.OnChanged(func(_ *nat.Mapping, _, _ []nat.Address) {
		t.notifyAddressesChanged(t)
	})
	defer t.natService.RemoveMapping(mapping)

	t.mut.Lock()
	t.mapping = mapping
	t.mut.Unlock()

	for {
		listener.SetDeadline(time.Now().Add(time.Second))
		conn, err := listener.Accept()
		select {
		case <-t.stop:
			if err == nil {
				conn.Close()
			}
			t.mut.Lock()
			t.mapping = nil
			t.mut.Unlock()
			return
		default:
		}
		if err != nil {
			if err, ok := err.(*net.OpError); !ok || !err.Timeout() {
				l.Warnln("Listen (BEP/tcp): Accepting connection:", err)
			}
			continue
		}

		l.Debugln("Listen (BEP/tcp): connect from", conn.RemoteAddr())

		if err := dialer.SetTCPOptions(conn); err != nil {
			l.Debugln("Listen (BEP/tcp): setting tcp options:", err)
		}

		if tc := t.cfg.Options().TrafficClass; tc != 0 {
			if err := dialer.SetTrafficClass(conn, tc); err != nil {
				l.Debugln("Listen (BEP/tcp): setting traffic class:", err)
			}
		}

		tc := tls.Server(conn, t.tlsCfg)
		if err := tlsTimedHandshake(tc); err != nil {
			l.Infoln("Listen (BEP/tcp): TLS handshake:", err)
			tc.Close()
			continue
		}

		t.conns <- internalConn{tc, connTypeTCPServer, tcpPriority}
	}
}

func (t *tcpListener) Stop() {
	close(t.stop)
}

func (t *tcpListener) URI() *url.URL {
	return t.uri
}

func (t *tcpListener) WANAddresses() []*url.URL {
	uris := t.LANAddresses()
	t.mut.RLock()
	if t.mapping != nil {
		addrs := t.mapping.ExternalAddresses()
		for _, addr := range addrs {
			uri := *t.uri
			// Does net.JoinHostPort internally
			uri.Host = addr.String()
			uris = append(uris, &uri)

			// For every address with a specified IP, add one without an IP,
			// just in case the specified IP is still internal (router behind DMZ).
			if len(addr.IP) != 0 && !addr.IP.IsUnspecified() {
				uri = *t.uri
				addr.IP = nil
				uri.Host = addr.String()
				uris = append(uris, &uri)
			}
		}
	}
	t.mut.RUnlock()
	return uris
}

func (t *tcpListener) LANAddresses() []*url.URL {
	return []*url.URL{t.uri}
}

func (t *tcpListener) Error() error {
	t.mut.RLock()
	err := t.err
	t.mut.RUnlock()
	return err
}

func (t *tcpListener) String() string {
	return t.uri.String()
}

func (t *tcpListener) Factory() listenerFactory {
	return t.factory
}

func (t *tcpListener) NATType() string {
	return "unknown"
}

type tcpListenerFactory struct{}

func (f *tcpListenerFactory) New(uri *url.URL, cfg *config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service) genericListener {
	return &tcpListener{
		uri:        fixupPort(uri, config.DefaultTCPPort),
		cfg:        cfg,
		tlsCfg:     tlsCfg,
		conns:      conns,
		natService: natService,
		stop:       make(chan struct{}),
		factory:    f,
	}
}

func (tcpListenerFactory) Enabled(cfg config.Configuration) bool {
	return true
}
