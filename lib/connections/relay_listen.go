// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/relay/client"
)

func init() {
	listeners["relay"] = newRelayListener
	listeners["dynamic+http"] = newRelayListener
	listeners["dynamic+https"] = newRelayListener
}

type relayListener struct {
	onAddressesChangedNotifier

	uri     *url.URL
	tlsCfg  *tls.Config
	stopped chan struct{}
	conns   chan IntermediateConnection

	err    error
	client client.RelayClient
	mut    sync.RWMutex
}

func (t *relayListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	clnt, err := client.NewClient(t.uri, t.tlsCfg.Certificates, nil, 10*time.Second)

	go clnt.Serve()

	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Warnln("listen (BEP/relay):", err)
		return
	}

	t.mut.Lock()
	t.client = clnt
	t.mut.Unlock()

	oldURI := clnt.URI()

	for {
		select {
		case inv, ok := <-t.client.Invitations():
			if !ok {
				close(t.stopped)
				return
			}

			conn, err := client.JoinSession(inv)
			if err != nil {
				l.Warnln("Joining relay session (BEP/relay):", err)
				continue
			}

			err = dialer.SetTCPOptions(conn.(*net.TCPConn))
			if err != nil {
				l.Infoln(err)
			}

			var tc *tls.Conn
			if inv.ServerSocket {
				tc = tls.Server(conn, t.tlsCfg)
			} else {
				tc = tls.Client(conn, t.tlsCfg)
			}

			err = tc.Handshake()
			if err != nil {
				tc.Close()
				l.Infoln("TLS handshake (BEP/relay):", err)
				continue
			}

			t.conns <- IntermediateConnection{tc, "Relay (Server)", relayPriority}

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
	var stopped chan struct{}

	t.mut.Lock()
	if t.client != nil {
		t.stopped = make(chan struct{})
		stopped = t.stopped
		t.client.Stop()
	}
	t.mut.Unlock()

	if stopped != nil {
		<-t.stopped
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

	return []*url.URL{client.URI()}
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

func (t *relayListener) String() string {
	return t.uri.String()
}

func newRelayListener(uri *url.URL, tlsCfg *tls.Config, conns chan IntermediateConnection, natService *nat.Service) genericListener {
	return &relayListener{
		uri:    uri,
		tlsCfg: tlsCfg,
		conns:  conns,
	}
}
