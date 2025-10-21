// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !noquic
// +build !noquic

package connections

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/stun"
	"github.com/syncthing/syncthing/lib/svcutil"
)

func init() {
	factory := &quicListenerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		listeners[scheme] = factory
	}
}

type quicListener struct {
	svcutil.ServiceWithError
	onAddressesChangedNotifier

	nat atomic.Uint64 // Holds a stun.NATType.

	uri        *url.URL
	cfg        config.Wrapper
	tlsCfg     *tls.Config
	conns      chan internalConn
	factory    listenerFactory
	registry   *registry.Registry
	lanChecker *lanChecker

	address    *url.URL
	natService *nat.Service
	mapping    *nat.Mapping
	laddr      net.Addr
	mut        sync.Mutex
}

func (t *quicListener) OnNATTypeChanged(natType stun.NATType) {
	if natType != stun.NATUnknown {
		slog.Info("Detected NAT type", slogutil.URI(t.uri), slog.Any("type", natType))
	}
	t.nat.Store(uint64(natType))
}

func (t *quicListener) OnExternalAddressChanged(address *stun.Host, via string) {
	var uri *url.URL
	if address != nil {
		copy := *t.uri
		uri = &copy
		uri.Host = address.TransportAddr()
	}

	t.mut.Lock()
	existingAddress := t.address
	t.address = uri
	t.mut.Unlock()

	if uri != nil && (existingAddress == nil || existingAddress.String() != uri.String()) {
		slog.Info("Resolved external address", slogutil.URI(t.uri), slogutil.Address(uri.String()), slog.String("via", via))
		t.notifyAddressesChanged(t)
	} else if uri == nil && existingAddress != nil {
		t.notifyAddressesChanged(t)
	}
}

func (t *quicListener) serve(ctx context.Context) error {
	network := quicNetwork(t.uri)

	udpAddr, err := net.ResolveUDPAddr(network, t.uri.Host)
	if err != nil {
		slog.WarnContext(ctx, "Failed to listen (QUIC)", slogutil.Error(err))
		return err
	}

	udpConn, err := net.ListenUDP(network, udpAddr)
	if err != nil {
		slog.WarnContext(ctx, "Failed to listen (QUIC)", slogutil.Error(err))
		return err
	}
	defer udpConn.Close()

	tracer := &writeTrackingTracer{}
	quicTransport := &quic.Transport{
		Conn:   udpConn,
		Tracer: tracer.loggingTracer(),
	}
	defer quicTransport.Close()

	svc := stun.New(t.cfg, t, &transportPacketConn{tran: quicTransport}, tracer)
	stunCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Serve(stunCtx)

	t.registry.Register(t.uri.Scheme, quicTransport)
	defer t.registry.Unregister(t.uri.Scheme, quicTransport)

	listener, err := quicTransport.Listen(t.tlsCfg, quicConfig)
	if err != nil {
		slog.WarnContext(ctx, "Failed to listen (QUIC)", slogutil.Error(err))
		return err
	}
	defer listener.Close()

	t.notifyAddressesChanged(t)
	defer t.clearAddresses(t)

	slog.InfoContext(ctx, "QUIC listener starting", slogutil.Address(udpConn.LocalAddr()))
	defer slog.InfoContext(ctx, "QUIC listener shutting down", slogutil.Address(udpConn.LocalAddr()))

	var ipVersion nat.IPVersion
	switch t.uri.Scheme {
	case "quic4":
		ipVersion = nat.IPv4Only
	case "quic6":
		ipVersion = nat.IPv6Only
	default:
		ipVersion = nat.IPvAny
	}
	mapping := t.natService.NewMapping(nat.UDP, ipVersion, udpAddr.IP, udpAddr.Port)
	mapping.OnChanged(func() {
		t.notifyAddressesChanged(t)
	})
	// Should be called after t.mapping is nil'ed out.
	defer t.natService.RemoveMapping(mapping)

	t.mut.Lock()
	t.mapping = mapping
	t.laddr = udpConn.LocalAddr()
	t.mut.Unlock()
	defer func() {
		t.mut.Lock()
		t.laddr = nil
		t.mut.Unlock()
	}()

	acceptFailures := 0
	const maxAcceptFailures = 10

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		session, err := listener.Accept(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		} else if err != nil {
			slog.WarnContext(ctx, "Failed to accept QUIC connection", slogutil.Error(err))

			acceptFailures++
			if acceptFailures > maxAcceptFailures {
				// Return to restart the listener, because something
				// seems permanently damaged.
				return err
			}

			// Slightly increased delay for each failure.
			time.Sleep(time.Duration(acceptFailures) * time.Second)

			continue
		}

		acceptFailures = 0

		slog.DebugContext(ctx, "Incoming connection", "from", session.RemoteAddr())

		streamCtx, cancel := context.WithTimeout(ctx, quicOperationTimeout)
		stream, err := session.AcceptStream(streamCtx)
		cancel()
		if err != nil {
			slog.DebugContext(ctx, "Failed to accept stream", slogutil.Address(session.RemoteAddr()), slogutil.Error(err))
			_ = session.CloseWithError(1, err.Error())
			continue
		}

		priority := t.cfg.Options().ConnectionPriorityQUICWAN
		isLocal := t.lanChecker.isLAN(session.RemoteAddr())
		if isLocal {
			priority = t.cfg.Options().ConnectionPriorityQUICLAN
		}
		t.conns <- newInternalConn(&quicTlsConn{session, stream, nil}, connTypeQUICServer, isLocal, priority)
	}
}

func (t *quicListener) URI() *url.URL {
	return t.uri
}

func (t *quicListener) WANAddresses() []*url.URL {
	t.mut.Lock()
	uris := []*url.URL{maybeReplacePort(t.uri, t.laddr)}
	if t.address != nil {
		uris = append(uris, t.address)
	}

	uris = append(uris, portMappingURIs(t.mapping, *t.uri)...)

	t.mut.Unlock()
	return uris
}

func (t *quicListener) LANAddresses() []*url.URL {
	t.mut.Lock()
	uri := maybeReplacePort(t.uri, t.laddr)
	t.mut.Unlock()
	addrs := []*url.URL{uri}
	network := quicNetwork(uri)
	addrs = append(addrs, getURLsForAllAdaptersIfUnspecified(network, uri)...)
	return addrs
}

func (t *quicListener) String() string {
	return t.uri.String()
}

func (t *quicListener) Factory() listenerFactory {
	return t.factory
}

func (t *quicListener) NATType() string {
	v := stun.NATType(t.nat.Load())
	if v == stun.NATUnknown || v == stun.NATError {
		return "unknown"
	}
	return v.String()
}

type quicListenerFactory struct{}

func (*quicListenerFactory) Valid(config.Configuration) error {
	return nil
}

func (f *quicListenerFactory) New(uri *url.URL, cfg config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service, registry *registry.Registry, lanChecker *lanChecker) genericListener {
	l := &quicListener{
		uri:        fixupPort(uri, config.DefaultQUICPort),
		cfg:        cfg,
		tlsCfg:     tlsCfg,
		conns:      conns,
		natService: natService,
		factory:    f,
		registry:   registry,
		lanChecker: lanChecker,
	}
	l.ServiceWithError = svcutil.AsService(l.serve, l.String())
	l.nat.Store(uint64(stun.NATUnknown))
	return l
}

func (quicListenerFactory) Enabled(_ config.Configuration) bool {
	return true
}
