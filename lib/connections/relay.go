// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay/client"
)

var (
	relayPriority = 200
)

func init() {
	dialers["relay"] = relayDialer{}
	listeners["relay"] = newRelayListener
	listeners["dynamic+http"] = newRelayListener
	listeners["dynamic+https"] = newRelayListener
}

type relayDialer struct{}

func (relayDialer) Dial(id protocol.DeviceID, uri *url.URL, tlsCfg *tls.Config) (IntermediateConnection, error) {
	inv, err := client.GetInvitationFromRelay(uri, id, tlsCfg.Certificates, 10*time.Second)
	if err != nil {
		return IntermediateConnection{}, err
	}

	conn, err := client.JoinSession(inv)
	if err != nil {
		return IntermediateConnection{}, err
	}

	err = osutil.SetTCPOptions(conn.(*net.TCPConn))
	if err != nil {
		conn.Close()
		return IntermediateConnection{}, err
	}

	var tc *tls.Conn
	if inv.ServerSocket {
		tc = tls.Server(conn, tlsCfg)
	} else {
		tc = tls.Client(conn, tlsCfg)
	}

	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return IntermediateConnection{}, err
	}

	return IntermediateConnection{tc, "relay-dial", relayPriority}, nil
}

func (relayDialer) Priority() int {
	return relayPriority
}

type relayListener struct {
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

	for inv := range t.client.Invitations() {
		conn, err := client.JoinSession(inv)
		if err != nil {
			l.Warnln("Joining relay session (BEP/relay):", err)
			continue
		}

		err = osutil.SetTCPOptions(conn.(*net.TCPConn))
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

		t.conns <- IntermediateConnection{tc, "relay-listen", relayPriority}
	}

	close(t.stopped)
}

func (t *relayListener) Stop() {
	t.stopped = make(chan struct{})

	t.mut.RLock()
	clnt := t.client
	t.mut.RUnlock()

	clnt.Stop()
	<-t.stopped
}

func (t *relayListener) WANAddresses() []*url.URL {
	t.mut.RLock()
	clnt := t.client
	t.mut.RUnlock()

	return []*url.URL{clnt.URI()}
}

func (t *relayListener) LANAddresses() []*url.URL {
	t.mut.RLock()
	clnt := t.client
	t.mut.RUnlock()

	return []*url.URL{clnt.URI()}
}

func (t *relayListener) Error() error {
	t.mut.RLock()
	err := t.err
	clnt := t.client
	t.mut.RUnlock()

	if err != nil {
		return err
	}
	if !clnt.StatusOK() {
		return fmt.Errorf("Relay failed to connect")
	}
	return nil
}

func (t *relayListener) Details() interface{} {
	// TODO: AUD implement
	return nil
}

func newRelayListener(uri *url.URL, tlsCfg *tls.Config, conns chan IntermediateConnection, natService *nat.Service) genericListener {
	return &relayListener{
		uri:    uri,
		tlsCfg: tlsCfg,
		conns:  conns,
	}
}
