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
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/svcutil"
)

func init() {
	factory := &relayListenerFactory{}
	listeners["relay"] = factory
	listeners["dynamic+http"] = factory
	listeners["dynamic+https"] = factory
}

type relayListener struct {
	svcutil.ServiceWithError
	onAddressesChangedNotifier

	uri     *url.URL
	cfg     config.Wrapper
	tlsCfg  *tls.Config
	conns   chan internalConn
	factory listenerFactory

	client client.RelayClient
	mut    sync.RWMutex
}

func (t *relayListener) serve(ctx context.Context) error {
	clnt, err := client.NewClient(t.uri, t.tlsCfg.Certificates, 10*time.Second)
	if err != nil {
		l.Infoln("Listen (BEP/relay):", err)
		return err
	}

	t.mut.Lock()
	t.client = clnt
	t.mut.Unlock()

	l.Infof("Relay listener (%v) starting", t)
	defer l.Infof("Relay listener (%v) shutting down", t)
	defer t.clearAddresses(t)

	invitationCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go t.handleInvitations(invitationCtx, clnt)

	return clnt.Serve(ctx)
}

func (t *relayListener) handleInvitations(ctx context.Context, clnt client.RelayClient) {
	invitations := clnt.Invitations()

	// Start with nil, so that we send a addresses changed notification as soon as we connect somewhere.
	var oldURI *url.URL

	for {
		select {
		case inv := <-invitations:
			conn, err := client.JoinSession(ctx, inv)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					l.Infoln("Listen (BEP/relay): joining session:", err)
				}
				continue
			}

			err = dialer.SetTCPOptions(conn)
			if err != nil {
				l.Debugln("Listen (BEP/relay): setting tcp options:", err)
			}

			err = dialer.SetTrafficClass(conn, t.cfg.Options().TrafficClass)
			if err != nil {
				l.Debugln("Listen (BEP/relay): setting traffic class:", err)
			}

			var tc *tls.Conn
			if inv.ServerSocket {
				tc = tls.Server(conn, t.tlsCfg)
			} else {
				tc = tls.Client(conn, t.tlsCfg)
			}

			err = tlsTimedHandshake(tc)
			if err != nil {
				tc.Close()
				l.Infoln("Listen (BEP/relay): TLS handshake:", err)
				continue
			}

			t.conns <- newInternalConn(tc, connTypeRelayServer, false, t.cfg.Options().ConnectionPriorityRelay)

		// Poor mans notifier that informs the connection service that the
		// relay URI has changed. This can only happen when we connect to a
		// relay via dynamic+http(s) pool, which upon a relay failing/dropping
		// us, would pick a different one.
		case <-time.After(10 * time.Second):
			currentURI := clnt.URI()
			if currentURI != oldURI {
				oldURI = currentURI
				t.notifyAddressesChanged(t)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (t *relayListener) URI() *url.URL {
	return t.uri
}

func (t *relayListener) WANAddresses() []*url.URL {
	t.mut.RLock()
	client := t.client
	t.mut.RUnlock()

	if client == nil {
		return nil
	}

	curi := client.URI()
	if curi == nil {
		return nil
	}

	return []*url.URL{curi}
}

func (t *relayListener) LANAddresses() []*url.URL {
	return t.WANAddresses()
}

func (t *relayListener) Error() error {
	err := t.ServiceWithError.Error()
	if err != nil {
		return err
	}
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.client != nil {
		return t.client.Error()
	}
	return nil
}

func (t *relayListener) Factory() listenerFactory {
	return t.factory
}

func (t *relayListener) String() string {
	return t.uri.String()
}

func (*relayListener) NATType() string {
	return "unknown"
}

type relayListenerFactory struct{}

func (f *relayListenerFactory) New(uri *url.URL, cfg config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, _ *nat.Service, _ *registry.Registry, _ *lanChecker) genericListener {
	t := &relayListener{
		uri:     uri,
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		factory: f,
	}
	t.ServiceWithError = svcutil.AsService(t.serve, t.String())
	return t
}

func (relayListenerFactory) Valid(cfg config.Configuration) error {
	if !cfg.Options.RelaysEnabled {
		return errDisabled
	}
	return nil
}
