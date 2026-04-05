// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/slogutil"
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

	uri        *url.URL
	cfg        config.Wrapper
	tlsCfg     *tls.Config
	conns      chan internalConn
	factory    listenerFactory
	registry   *registry.Registry
	lanChecker *lanChecker

	natService *nat.Service
	mapping    *nat.Mapping
	laddr      net.Addr

	mut sync.RWMutex
}

func (t *tcpListener) serve(ctx context.Context) error {
	tcaddr, err := net.ResolveTCPAddr(t.uri.Scheme, t.uri.Host)
	if err != nil {
		slog.WarnContext(ctx, "Failed to listen (TCP)", slogutil.Error(err))
		return err
	}

	lc := net.ListenConfig{
		Control: dialer.ReusePortControl,
	}

	listener, err := lc.Listen(context.TODO(), t.uri.Scheme, tcaddr.String())
	if err != nil {
		slog.WarnContext(ctx, "Failed to listen (TCP)", slogutil.Error(err))
		return err
	}
	defer listener.Close()

	// We might bind to :0, so use the port we've been given.
	tcaddr = listener.Addr().(*net.TCPAddr)

	t.notifyAddressesChanged(t)
	defer t.clearAddresses(t)

	t.registry.Register(t.uri.Scheme, tcaddr)
	defer t.registry.Unregister(t.uri.Scheme, tcaddr)

	slog.InfoContext(ctx, "TCP listener starting", slogutil.Address(tcaddr))
	defer slog.InfoContext(ctx, "TCP listener shutting down", slogutil.Address(tcaddr))

	var ipVersion nat.IPVersion
	switch t.uri.Scheme {
	case "tcp4":
		ipVersion = nat.IPv4Only
	case "tcp6":
		ipVersion = nat.IPv6Only
	default:
		ipVersion = nat.IPvAny
	}
	mapping := t.natService.NewMapping(nat.TCP, ipVersion, tcaddr.IP, tcaddr.Port)
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
			var ne *net.OpError
			if ok := errors.As(err, &ne); !ok || !ne.Timeout() {
				slog.WarnContext(ctx, "Failed to accept TCP connection", slogutil.Error(err))

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
			slog.WarnContext(ctx, "Failed TLS handshake", slogutil.Address(tc.RemoteAddr()), slogutil.Error(err))
			tc.Close()
			continue
		}

		priority := t.cfg.Options().ConnectionPriorityTCPWAN
		isLocal := t.lanChecker.isLAN(conn.RemoteAddr())
		if isLocal {
			priority = t.cfg.Options().ConnectionPriorityTCPLAN
		}
		t.conns <- newInternalConn(tc, connTypeTCPServer, isLocal, priority)
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

	uris = append(uris, portMappingURIs(t.mapping, *t.uri)...)

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

func (*tcpListener) NATType() string {
	return "unknown"
}

type tcpListenerFactory struct{}

func (f *tcpListenerFactory) New(uri *url.URL, cfg config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service, registry *registry.Registry, lanChecker *lanChecker) genericListener {
	l := &tcpListener{
		uri:        fixupPort(uri, config.DefaultTCPPort),
		cfg:        cfg,
		tlsCfg:     tlsCfg,
		conns:      conns,
		natService: natService,
		factory:    f,
		registry:   registry,
		lanChecker: lanChecker,
	}
	l.ServiceWithError = svcutil.AsService(l.serve, l.String())
	return l
}

func (tcpListenerFactory) Valid(_ config.Configuration) error {
	// Always valid
	return nil
}
