// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.12

package connections

import (
	"crypto/tls"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucas-clemente/quic-go"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/stun"
)

func init() {
	factory := &quicListenerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		listeners[scheme] = factory
	}
}

type quicListener struct {
	nat atomic.Value

	onAddressesChangedNotifier

	uri     *url.URL
	cfg     config.Wrapper
	tlsCfg  *tls.Config
	stop    chan struct{}
	conns   chan internalConn
	factory listenerFactory

	address *url.URL
	err     error
	mut     sync.Mutex
}

func (t *quicListener) OnNATTypeChanged(natType stun.NATType) {
	if natType != stun.NATUnknown {
		l.Infof("%s detected NAT type: %s", t.uri, natType)
	}
	t.nat.Store(natType)
}

func (t *quicListener) OnExternalAddressChanged(address *stun.Host, via string) {
	var uri *url.URL
	if address != nil {
		uri = &(*t.uri)
		uri.Host = address.TransportAddr()
	}

	t.mut.Lock()
	existingAddress := t.address
	t.address = uri
	t.mut.Unlock()

	if uri != nil && (existingAddress == nil || existingAddress.String() != uri.String()) {
		l.Infof("%s resolved external address %s (via %s)", t.uri, uri.String(), via)
		t.notifyAddressesChanged(t)
	} else if uri == nil && existingAddress != nil {
		t.notifyAddressesChanged(t)
	}
}

func (t *quicListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	network := strings.Replace(t.uri.Scheme, "quic", "udp", -1)

	packetConn, err := net.ListenPacket(network, t.uri.Host)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("Listen (BEP/quic):", err)
		return
	}
	defer func() { _ = packetConn.Close() }()

	svc, conn := stun.New(t.cfg, t, packetConn)
	defer func() { _ = conn.Close() }()

	go svc.Serve()
	defer svc.Stop()

	registry.Register(t.uri.Scheme, conn)
	defer registry.Unregister(t.uri.Scheme, conn)

	listener, err := quic.Listen(conn, t.tlsCfg, quicConfig)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("Listen (BEP/quic):", err)
		return
	}

	l.Infof("QUIC listener (%v) starting", packetConn.LocalAddr())
	defer l.Infof("QUIC listener (%v) shutting down", packetConn.LocalAddr())

	// Accept is forever, so handle stops externally.
	go func() {
		select {
		case <-t.stop:
			_ = listener.Close()
		}
	}()

	for {
		// Blocks forever, see https://github.com/lucas-clemente/quic-go/issues/1915
		session, err := listener.Accept()

		select {
		case <-t.stop:
			if err == nil {
				_ = session.Close()
			}
			return
		default:
		}
		if err != nil {
			if err, ok := err.(net.Error); !ok || !err.Timeout() {
				l.Warnln("Listen (BEP/quic): Accepting connection:", err)
			}
			continue
		}

		l.Debugln("connect from", session.RemoteAddr())

		// Accept blocks forever, give it 10s to do it's thing.
		ok := make(chan struct{})
		go func() {
			select {
			case <-ok:
				return
			case <-t.stop:
				_ = session.Close()
			case <-time.After(10 * time.Second):
				l.Debugln("timed out waiting for AcceptStream on", session.RemoteAddr())
				_ = session.Close()
			}
		}()

		stream, err := session.AcceptStream()
		close(ok)
		if err != nil {
			l.Debugln("failed to accept stream from", session.RemoteAddr(), err.Error())
			_ = session.Close()
			continue
		}

		t.conns <- internalConn{&quicTlsConn{session, stream}, connTypeQUICServer, quicPriority}
	}
}

func (t *quicListener) Stop() {
	close(t.stop)
}

func (t *quicListener) URI() *url.URL {
	return t.uri
}

func (t *quicListener) WANAddresses() []*url.URL {
	uris := t.LANAddresses()
	t.mut.Lock()
	if t.address != nil {
		uris = append(uris, t.address)
	}
	t.mut.Unlock()
	return uris
}

func (t *quicListener) LANAddresses() []*url.URL {
	return []*url.URL{t.uri}
}

func (t *quicListener) Error() error {
	t.mut.Lock()
	err := t.err
	t.mut.Unlock()
	return err
}

func (t *quicListener) String() string {
	return t.uri.String()
}

func (t *quicListener) Factory() listenerFactory {
	return t.factory
}

func (t *quicListener) NATType() string {
	v := t.nat.Load().(stun.NATType)
	if v == stun.NATUnknown || v == stun.NATError {
		return "unknown"
	}
	return v.String()
}

type quicListenerFactory struct{}

func (f *quicListenerFactory) Valid(config.Configuration) error {
	return nil
}

func (f *quicListenerFactory) New(uri *url.URL, cfg config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service) genericListener {
	l := &quicListener{
		uri:     fixupPort(uri, config.DefaultQUICPort),
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		stop:    make(chan struct{}),
		factory: f,
	}
	l.nat.Store(stun.NATUnknown)
	return l
}

func (quicListenerFactory) Enabled(cfg config.Configuration) bool {
	return true
}
