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
	"strings"
	"sync"
	"time"

	"github.com/ccding/go-stun/stun"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/utp"
)

func init() {
	factory := &utpListenerFactory{}
	for _, scheme := range []string{"utp", "utp4", "utp6"} {
		listeners[scheme] = factory
	}
}

type utpListener struct {
	onAddressesChangedNotifier

	uri     *url.URL
	cfg     *config.Wrapper
	tlsCfg  *tls.Config
	stop    chan struct{}
	conns   chan IntermediateConnection
	factory listenerFactory

	address *url.URL
	err     error
	mut     sync.RWMutex
}

func (t *utpListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	network := strings.Replace(t.uri.Scheme, "utp", "udp", -1)

	listener, err := utp.NewSocket(network, t.uri.Host)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("listen (BEP/utp):", err)
		return
	}
	defer listener.Close()

	l.Infof("UTP listener (%v) starting", listener.Addr())
	defer l.Infof("UTP listener (%v) shutting down", listener.Addr())

	go t.stunRenewal(listener)

	for {
		listener.SetDeadline(time.Now().Add(time.Second))
		conn, err := listener.Accept()
		select {
		case <-t.stop:
			if err == nil {
				conn.Close()
			}
			return
		default:
		}
		if err != nil {
			if err, ok := err.(*net.OpError); !ok || !err.Timeout() {
				l.Warnln("Accepting connection (BEP/utp):", err)
			}
			continue
		}

		l.Debugln("connect from", conn.RemoteAddr())

		tc := tls.Server(conn, t.tlsCfg)
		err = tc.Handshake()
		if err != nil {
			l.Infoln("TLS handshake (BEP/utp):", err)
			tc.Close()
			continue
		}

		t.conns <- IntermediateConnection{tc, "UTP (Server)", utpPriority}
	}
}

func (t *utpListener) Stop() {
	close(t.stop)
}

func (t *utpListener) URI() *url.URL {
	return t.uri
}

func (t *utpListener) WANAddresses() []*url.URL {
	uris := t.LANAddresses()
	t.mut.RLock()
	if t.address != nil {
		uris = append(uris, t.address)
	}
	t.mut.RUnlock()
	return uris
}

func (t *utpListener) LANAddresses() []*url.URL {
	return []*url.URL{t.uri}
}

func (t *utpListener) Error() error {
	t.mut.RLock()
	err := t.err
	t.mut.RUnlock()
	return err
}

func (t *utpListener) String() string {
	return t.uri.String()
}

func (t *utpListener) Factory() listenerFactory {
	return t.factory
}

func (t *utpListener) stunRenewal(listener *utp.Socket) {
	oldType := stun.NAT_UNKNOWN
	for {
		client := stun.NewClientWithConnection(listener)
		client.SetSoftwareName("syncthing")

		var uri url.URL
		var natType = stun.NAT_UNKNOWN
		var extAddr *stun.Host
		var err error

		for _, addr := range t.cfg.StunServers() {
			client.SetServerAddr(addr)

			natType, extAddr, err = client.Discover()
			if err != nil || extAddr == nil {
				l.Debugf("%s stun discovery on %s: %s (%s)", t.uri, addr, err, extAddr)
				continue
			}

			uri = *t.uri
			uri.Host = extAddr.TransportAddr()

			t.mut.Lock()
			changed := false
			if oldType != natType || t.address.String() != uri.String() {
				l.Infof("%s detected NAT type: %s, external address: %s", t.uri, natType, uri.String())
			}

			if t.address == nil || t.address.String() != uri.String() {
				t.address = &uri
				changed = true
			}

			t.mut.Unlock()

			// This will most likely result in a call to URI() which tries to
			// get t.mut, so notify while unlocked.
			if changed {
				t.notifyAddressesChanged(t)
			}

			break
		}

		oldType = natType

		select {
		case <-time.After(time.Duration(t.cfg.Options().StunRenewalM) * time.Minute):
		case <-t.stop:
			return

		}
	}
}

type utpListenerFactory struct{}

func (f *utpListenerFactory) New(uri *url.URL, cfg *config.Wrapper, tlsCfg *tls.Config, conns chan IntermediateConnection, natService *nat.Service) genericListener {
	return &utpListener{
		uri:     fixupPort(uri, 22000),
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		stop:    make(chan struct{}),
		factory: f,
	}
}

func (utpListenerFactory) Enabled(cfg config.Configuration) bool {
	return true
}
