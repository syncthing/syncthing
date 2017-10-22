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
	"sync/atomic"
	"time"

	"github.com/AudriusButkevicius/kcp-go"
	"github.com/AudriusButkevicius/pfilter"
	"github.com/ccding/go-stun/stun"
	"github.com/xtaci/smux"

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
	conns   chan internalConn
	factory listenerFactory
	nat     atomic.Value

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
	kcpConn := filterConn.NewConn(kcpNoFilterPriority, nil)
	stunConn := filterConn.NewConn(kcpStunFilterPriority, &stunFilter{
		ids: make(map[string]time.Time),
	})

	filterConn.Start()
	registerFilter(filterConn)

	listener, err := kcp.ServeConn(nil, 0, 0, kcpConn)
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
	defer deregisterFilter(filterConn)
	defer packetConn.Close()

	l.Infof("KCP listener (%v) starting", kcpConn.LocalAddr())
	defer l.Infof("KCP listener (%v) shutting down", kcpConn.LocalAddr())

	go t.stunRenewal(stunConn)

	for {
		listener.SetDeadline(time.Now().Add(time.Second))
		conn, err := listener.AcceptKCP()

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

		opts := t.cfg.Options()

		conn.SetStreamMode(true)
		conn.SetACKNoDelay(false)
		conn.SetWindowSize(opts.KCPSendWindowSize, opts.KCPReceiveWindowSize)
		conn.SetNoDelay(boolInt(opts.KCPNoDelay), opts.KCPUpdateIntervalMs, boolInt(opts.KCPFastResend), boolInt(!opts.KCPCongestionControl))

		l.Debugln("connect from", conn.RemoteAddr())

		ses, err := smux.Server(conn, smuxConfig)
		if err != nil {
			l.Debugln("smux server:", err)
			conn.Close()
			continue
		}

		ses.SetDeadline(time.Now().Add(10 * time.Second))
		stream, err := ses.AcceptStream()
		if err != nil {
			l.Debugln("smux accept:", err)
			ses.Close()
			continue
		}
		ses.SetDeadline(time.Time{})

		tc := tls.Server(&sessionClosingStream{stream, ses}, t.tlsCfg)
		tc.SetDeadline(time.Now().Add(time.Second * 10))
		err = tc.Handshake()
		if err != nil {
			l.Debugln("TLS handshake (BEP/kcp):", err)
			tc.Close()
			continue
		}
		tc.SetDeadline(time.Time{})

		t.conns <- internalConn{tc, connTypeKCPServer, kcpPriority}
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

func (t *kcpListener) NATType() string {
	v := t.nat.Load().(stun.NATType)
	if v == stun.NATUnknown || v == stun.NATError {
		return "unknown"
	}
	return v.String()
}

func (t *kcpListener) stunRenewal(listener net.PacketConn) {
	client := stun.NewClientWithConnection(listener)
	client.SetSoftwareName("syncthing")

	var natType stun.NATType
	var extAddr *stun.Host
	var udpAddr *net.UDPAddr
	var err error

	oldType := stun.NATUnknown

	for {

	disabled:
		if t.cfg.Options().StunKeepaliveS < 1 {
			time.Sleep(time.Second)
			oldType = stun.NATUnknown
			t.nat.Store(stun.NATUnknown)
			t.mut.Lock()
			t.address = nil
			t.mut.Unlock()
			continue
		}

		for _, addr := range t.cfg.StunServers() {
			// Resolve the address, so that in case the server advertises two
			// IPs, we always hit the same one, as otherwise, the mapping might
			// expire as we hit the other address, and cause us to flip flop
			// between servers/external addresses, as a result flooding discovery
			// servers.
			udpAddr, err = net.ResolveUDPAddr("udp", addr)
			if err != nil {
				l.Debugf("%s stun addr resolution on %s: %s", t.uri, addr, err)
				continue
			}
			client.SetServerAddr(udpAddr.String())

			natType, extAddr, err = client.Discover()
			if err != nil || extAddr == nil {
				l.Debugf("%s stun discovery on %s: %s", t.uri, addr, err)
				continue
			}

			// The stun server is most likely borked, try another one.
			if natType == stun.NATError || natType == stun.NATUnknown || natType == stun.NATBlocked {
				l.Debugf("%s stun discovery on %s resolved to %s", t.uri, addr, natType)
				continue
			}

			if oldType != natType {
				l.Infof("%s detected NAT type: %s", t.uri, natType)
				t.nat.Store(natType)
			}

			for {
				changed := false

				uri := *t.uri
				uri.Host = extAddr.TransportAddr()

				t.mut.Lock()

				if t.address == nil || t.address.String() != uri.String() {
					l.Infof("%s resolved external address %s (via %s)", t.uri, uri.String(), addr)
					t.address = &uri
					changed = true
				}
				t.mut.Unlock()

				// This will most likely result in a call to WANAddresses() which tries to
				// get t.mut, so notify while unlocked.
				if changed {
					t.notifyAddressesChanged(t)
				}

				select {
				case <-time.After(time.Duration(t.cfg.Options().StunKeepaliveS) * time.Second):
				case <-t.stop:
					return
				}

				if t.cfg.Options().StunKeepaliveS < 1 {
					goto disabled
				}

				extAddr, err = client.Keepalive()
				if err != nil {
					l.Debugf("%s stun keepalive on %s: %s (%v)", t.uri, addr, err, extAddr)
					break
				}
			}

			oldType = natType
		}

		// We failed to contact all provided stun servers, chillout for a while.
		time.Sleep(time.Minute)
	}
}

type kcpListenerFactory struct{}

func (f *kcpListenerFactory) New(uri *url.URL, cfg *config.Wrapper, tlsCfg *tls.Config, conns chan internalConn, natService *nat.Service) genericListener {
	l := &kcpListener{
		uri:     fixupPort(uri, config.DefaultKCPPort),
		cfg:     cfg,
		tlsCfg:  tlsCfg,
		conns:   conns,
		stop:    make(chan struct{}),
		factory: f,
	}
	l.nat.Store(stun.NATUnknown)
	return l
}

func (kcpListenerFactory) Enabled(cfg config.Configuration) bool {
	return true
}
