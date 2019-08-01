// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/util"
)

func init() {
	factory := &relayListenerFactory{}
	listeners["relay"] = factory
	listeners["dynamic+http"] = factory
	listeners["dynamic+https"] = factory
}

type relayListener struct {
	util.ServiceWithError
	onAddressesChangedNotifier

	uri     *url.URL
	cfg     config.Wrapper
	tlsCfg  *tls.Config
	conns   chan internalConn
	factory listenerFactory

	client client.RelayClient
	mut    sync.RWMutex
}

func (t *relayListener) serve(stop chan struct{}) error {
	clnt, err := client.NewClient(t.uri, t.tlsCfg.Certificates, nil, 10*time.Second)
	if err != nil {
		l.Infoln("Listen (BEP/relay):", err)
		return err
	}
	invitations := clnt.Invitations()

	t.mut.Lock()
	t.client = clnt
	go clnt.Serve()
	defer clnt.Stop()
	t.mut.Unlock()

	oldURI := clnt.URI()

	l.Infof("Relay listener (%v) starting", t)
	defer l.Infof("Relay listener (%v) shutting down", t)

	for {
		select {
		case inv, ok := <-invitations:
			if !ok {
				if err := clnt.Error(); err != nil {
					l.Infoln("Listen (BEP/relay):", err)
				}
				return err
			}

			conn, err := client.JoinSession(inv)
			if err != nil {
				l.Infoln("Listen (BEP/relay): joining session:", err)
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

			t.conns <- internalConn{tc, connTypeRelayServer, relayPriority}

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

		case <-stop:
			return nil
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

func (t *relayListener) NATType() string {
	return "unknown"
}

type relayListenerFactory struct{}

func (f *relayListenerFactory) New(uri *url.URL, cfg config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service) genericListener {
	t := &relayListener{
		uri:     uri,
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		factory: f,
	}
	t.ServiceWithError = util.AsServiceWithError(t.serve)
	return t
}

func (relayListenerFactory) Valid(cfg config.Configuration) error {
	if !cfg.Options.RelaysEnabled {
		return errDisabled
	}
	return nil
}
