// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AudriusButkevicius/kcp-go"
	"github.com/AudriusButkevicius/pfilter"
	"github.com/ccding/go-stun/stun"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/nat"
)

func init() {
	factory := &kcpListenerFactory{}
	for _, scheme := range []string{"kcp", "kcp4", "kcp6"} {
		listeners[scheme] = factory
	}
}

type kcpListener struct {
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

func (t *kcpListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	network := strings.Replace(t.uri.Scheme, "kcp", "udp", -1)

	packetConn, err := net.ListenPacket(network, t.uri.Host)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("listen (BEP/kcp):", err)
		return
	}
	filterConn := pfilter.NewPacketFilter(packetConn)
	kcpConn := filterConn.NewConn(1, nil)
	stunConn := filterConn.NewConn(0, &StunFilter{
		ids: make(map[string]struct{}),
	})

	filterConn.Start()

	listener, err := kcp.Listen(kcpConn, kcpLogger)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("listen (BEP/kcp):", err)
		return
	}

	defer listener.Close()
	defer stunConn.Close()
	defer kcpConn.Close()
	defer packetConn.Close()

	l.Infof("KCP listener (%v) starting", kcpConn.LocalAddr())
	defer l.Infof("KCP listener (%v) shutting down", kcpConn.LocalAddr())

	go t.stunRenewal(stunConn)

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
			if err, ok := err.(net.Error); !ok || !err.Timeout() {
				l.Warnln("Accepting connection (BEP/kcp):", err)
			}
			continue
		}

		conn.SetWindowSize(128, 128)
		conn.SetNoDelay(1, 10, 2, 1)
		conn.SetKeepAlive(0) // we do our own keep-alive in the stun routine

		l.Debugln("connect from", conn.RemoteAddr())

		conn.SetDeadline(time.Now().Add(time.Second * 10))
		tc := tls.Server(conn, t.tlsCfg)
		err = tc.Handshake()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && (nerr.Timeout() || nerr.Temporary()) {
				l.Debugln("TLS handshake (BEP/kcp) timeout", nerr)
			} else {
				l.Infoln("TLS handshake (BEP/kcp):", err)
			}
			tc.Close()
			continue
		}
		conn.SetDeadline(time.Time{})

		t.conns <- IntermediateConnection{tc, "KCP (Server)", kcpPriority}
	}
}

func (t *kcpListener) Stop() {
	close(t.stop)
}

func (t *kcpListener) URI() *url.URL {
	return t.uri
}

func (t *kcpListener) WANAddresses() []*url.URL {
	uris := t.LANAddresses()
	t.mut.RLock()
	if t.address != nil {
		uris = append(uris, t.address)
	}
	t.mut.RUnlock()
	return uris
}

func (t *kcpListener) LANAddresses() []*url.URL {
	return []*url.URL{t.uri}
}

func (t *kcpListener) Error() error {
	t.mut.RLock()
	err := t.err
	t.mut.RUnlock()
	return err
}

func (t *kcpListener) String() string {
	return t.uri.String()
}

func (t *kcpListener) Factory() listenerFactory {
	return t.factory
}

func (t *kcpListener) stunRenewal(listener net.PacketConn) {
	oldType := stun.NATUnknown
	for {
		client := stun.NewClientWithConnection(listener)
		client.SetSoftwareName("syncthing")

		var uri url.URL
		var natType = stun.NATUnknown
		var extAddr *stun.Host
		var err error

		for _, addr := range t.cfg.StunServers() {
			client.SetServerAddr(addr)

			natType, extAddr, err = client.Discover()
			if err != nil || extAddr == nil {
				l.Debugf("%s stun discovery on %s: %s (%v)", t.uri, addr, err, extAddr)
				continue
			}

			t.mut.Lock()
			if oldType != natType {
				l.Infof("%s detected NAT type: %s", t.uri, natType)
			}
			t.mut.Unlock()

			for {
				changed := false
				uri = *t.uri
				uri.Host = extAddr.TransportAddr()

				t.mut.Lock()
				if t.address == nil || t.address.String() != uri.String() {
					l.Infof("%s resolved external address %s", t.uri, uri.String())
					t.address = &uri
					changed = true
				}
				t.mut.Unlock()

				// This will most likely result in a call to URI() which tries to
				// get t.mut, so notify while unlocked.
				if changed {
					t.notifyAddressesChanged(t)
				}

				select {
				case <-time.After(time.Duration(t.cfg.Options().StunKeepaliveS) * time.Second):
				case <-t.stop:
					return
				}

				extAddr, err = client.Keepalive()
				if err != nil {
					break
				}
			}

			oldType = natType
		}
	}
}

type kcpListenerFactory struct{}

func (f *kcpListenerFactory) New(uri *url.URL, cfg *config.Wrapper, tlsCfg *tls.Config, conns chan IntermediateConnection, natService *nat.Service) genericListener {
	return &kcpListener{
		uri:     fixupPort(uri, 22000),
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		stop:    make(chan struct{}),
		factory: f,
	}
}

func (kcpListenerFactory) Enabled(cfg config.Configuration) bool {
	return true
}

type StunFilter struct {
	ids map[string]struct{}
}

func (f *StunFilter) Outgoing(out []byte, addr net.Addr) {
	if f.isStunPayload(out) {
		id := string(out[8:20])
		f.ids[id] = struct{}{}
	}
}

func (f *StunFilter) ClaimIncoming(in []byte, addr net.Addr) bool {
	if f.isStunPayload(in) {
		id := string(in[8:20])
		_, ok := f.ids[id]
		if ok {
			delete(f.ids, id)
		}
		return ok
	}
	return false
}

func (StunFilter) isStunPayload(data []byte) bool {
	// First two bits always unset, and should always send magic cookie.
	return data[0]&0xc0 == 0 && bytes.Equal(data[4:8], []byte{0x21, 0x12, 0xA4, 0x42})
}
