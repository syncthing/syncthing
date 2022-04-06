// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/svcutil"
)

func init() {
	factory := &tcpListenerFactory{}
	for _, scheme := range []string{"tcp", "tcp4", "tcp6"} {
		listeners[scheme] = factory
	}
}

type tcpListener struct {
	svcutil.ServiceWithError
	onAddressesChangedNotifier

	uri      *url.URL
	cfg      config.Wrapper
	tlsCfg   *tls.Config
	conns    chan internalConn
	factory  listenerFactory
	registry *registry.Registry

	natService *nat.Service
	mapping    *nat.Mapping
	laddr      net.Addr

	mut sync.RWMutex
}

func (t *tcpListener) serve(ctx context.Context) error {
	tcaddr, err := net.ResolveTCPAddr(t.uri.Scheme, t.uri.Host)
	if err != nil {
		l.Infoln("Listen (BEP/tcp):", err)
		return err
	}

	lc := net.ListenConfig{
		Control: dialer.ReusePortControl,
	}

	listener, err := lc.Listen(context.TODO(), t.uri.Scheme, tcaddr.String())
	if err != nil {
		l.Infoln("Listen (BEP/tcp):", err)
		return err
	}
	defer listener.Close()

	// We might bind to :0, so use the port we've been given.
	tcaddr = listener.Addr().(*net.TCPAddr)

	t.notifyAddressesChanged(t)
	defer t.clearAddresses(t)

	t.registry.Register(t.uri.Scheme, tcaddr)
	defer t.registry.Unregister(t.uri.Scheme, tcaddr)

	l.Infof("TCP listener (%v) starting", tcaddr)
	defer l.Infof("TCP listener (%v) shutting down", tcaddr)

	mapping := t.natService.NewMapping(nat.TCP, tcaddr.IP, tcaddr.Port)
	mapping.OnChanged(func() {
		t.notifyAddressesChanged(t)
	})
	// Should be called after t.mapping is nil'ed out.
	defer t.natService.RemoveMapping(mapping)

	t.mut.Lock()
	t.mapping = mapping
	t.laddr = tcaddr
	t.mut.Unlock()
	defer func() {
		t.mut.Lock()
		t.mapping = nil
		t.laddr = nil
		t.mut.Unlock()
	}()

	acceptFailures := 0
	const maxAcceptFailures = 10

	// :(, but what can you do.
	tcpListener := listener.(*net.TCPListener)

	for {
		_ = tcpListener.SetDeadline(time.Now().Add(time.Second))
		conn, err := tcpListener.Accept()
		select {
		case <-ctx.Done():
			if err == nil {
				conn.Close()
			}
			return nil
		default:
		}
		if err != nil {
			if err, ok := err.(*net.OpError); !ok || !err.Timeout() {
				l.Warnln("Listen (BEP/tcp): Accepting connection:", err)

				acceptFailures++
				if acceptFailures > maxAcceptFailures {
					// Return to restart the listener, because something
					// seems permanently damaged.
					return err
				}

				// Slightly increased delay for each failure.
				time.Sleep(time.Duration(acceptFailures) * time.Second)
			}
			continue
		}

		acceptFailures = 0
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

		t.conns <- newInternalConn(tc, connTypeTCPServer, tcpPriority)
	}
}

func (t *tcpListener) URI() *url.URL {
	return t.uri
}

func (t *tcpListener) WANAddresses() []*url.URL {
	t.mut.RLock()
	uris := []*url.URL{
		maybeReplacePort(t.uri, t.laddr),
	}
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

	// If we support ReusePort, add an unspecified zero port address, which will be resolved by the discovery server
	// in hopes that TCP punch through works.
	if dialer.SupportsReusePort {
		uri := *t.uri
		uri.Host = "0.0.0.0:0"
		uris = append([]*url.URL{&uri}, uris...)
	}
	return uris
}

func (t *tcpListener) LANAddresses() []*url.URL {
	t.mut.RLock()
	uri := maybeReplacePort(t.uri, t.laddr)
	t.mut.RUnlock()
	addrs := []*url.URL{uri}
	addrs = append(addrs, getURLsForAllAdaptersIfUnspecified(uri.Scheme, uri)...)
	return addrs
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

func (f *tcpListenerFactory) New(uri *url.URL, cfg config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service, registry *registry.Registry) genericListener {
	l := &tcpListener{
		uri:        fixupPort(uri, config.DefaultTCPPort),
		cfg:        cfg,
		tlsCfg:     tlsCfg,
		conns:      conns,
		natService: natService,
		factory:    f,
		registry:   registry,
	}
	l.ServiceWithError = svcutil.AsService(l.serve, l.String())
	return l
}

func (tcpListenerFactory) Valid(_ config.Configuration) error {
	// Always valid
	return nil
}
