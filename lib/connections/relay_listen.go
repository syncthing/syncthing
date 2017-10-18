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
)

func init() {
	factory := &relayListenerFactory{}
	listeners["relay"] = factory
	listeners["dynamic+http"] = factory
	listeners["dynamic+https"] = factory
}

type relayListener struct {
	onAddressesChangedNotifier

	uri     *url.URL
	cfg     *config.Wrapper
	tlsCfg  *tls.Config
	conns   chan internalConn
	factory listenerFactory

	err    error
	client client.RelayClient
	mut    sync.RWMutex
}

func (t *relayListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	clnt, err := client.NewClient(t.uri, t.tlsCfg.Certificates, nil, 10*time.Second)
	invitations := clnt.Invitations()
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Warnln("listen (BEP/relay):", err)
		return
	}

	go clnt.Serve()

	t.mut.Lock()
	t.client = clnt
	t.mut.Unlock()

	oldURI := clnt.URI()

	for {
		select {
		case inv, ok := <-invitations:
			if !ok {
				return
			}

			conn, err := client.JoinSession(inv)
			if err != nil {
				l.Infoln("Joining relay session (BEP/relay):", err)
				continue
			}

			err = dialer.SetTCPOptions(conn)
			if err != nil {
				l.Infoln(err)
			}

			err = dialer.SetTrafficClass(conn, t.cfg.Options().TrafficClass)
			if err != nil {
				l.Debugf("failed to set traffic class: %s", err)
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
				l.Infoln("TLS handshake (BEP/relay):", err)
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
		}
	}
}

func (t *relayListener) Stop() {
	t.mut.RLock()
	if t.client != nil {
		t.client.Stop()
	}
	t.mut.RUnlock()
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
	t.mut.RLock()
	err := t.err
	var cerr error
	if t.client != nil {
		cerr = t.client.Error()
	}
	t.mut.RUnlock()

	if err != nil {
		return err
	}
	return cerr
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

func (f *relayListenerFactory) New(uri *url.URL, cfg *config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service) genericListener {
	return &relayListener{
		uri:     uri,
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		factory: f,
	}
}

func (relayListenerFactory) Enabled(cfg config.Configuration) bool {
	return cfg.Options.RelaysEnabled
}
