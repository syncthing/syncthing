// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build go1.14,!noquic,!go1.17

package connections

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lucas-clemente/quic-go"

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
	nat atomic.Value

	onAddressesChangedNotifier

	uri     *url.URL
	cfg     config.Wrapper
	tlsCfg  *tls.Config
	conns   chan internalConn
	factory listenerFactory

	address *url.URL
	laddr   net.Addr
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
		copy := *t.uri
		uri = &copy
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

func (t *quicListener) serve(ctx context.Context) error {
	network := quicNetwork(t.uri)

	udpAddr, err := net.ResolveUDPAddr(network, t.uri.Host)
	if err != nil {
		l.Infoln("Listen (BEP/quic):", err)
		return err
	}

	udpConn, err := net.ListenUDP(network, udpAddr)
	if err != nil {
		l.Infoln("Listen (BEP/quic):", err)
		return err
	}
	defer func() { _ = udpConn.Close() }()

	svc, conn := stun.New(t.cfg, t, udpConn)
	defer conn.Close()

	quicWrapper := quicWrapper{
		PacketConn: conn,
		underlying: udpConn,
	}
	var wrapped net.PacketConn = &quicWrapper

	if oobConn, ok := conn.(oobConn); ok {
		l.Debugf("wrapping in oob conn")
		wrapped = &oobConnWrapper{
			quicWrapper, oobConn,
		}
	}

	go svc.Serve(ctx)

	registry.Register(t.uri.Scheme, wrapped)
	defer registry.Unregister(t.uri.Scheme, wrapped)

	listener, err := quic.Listen(wrapped, t.tlsCfg, quicConfig)
	if err != nil {
		l.Infoln("Listen (BEP/quic):", err)
		return err
	}
	defer listener.Close()

	t.notifyAddressesChanged(t)
	defer t.clearAddresses(t)

	l.Infof("QUIC listener (%v) starting", udpConn.LocalAddr())
	defer l.Infof("QUIC listener (%v) shutting down", udpConn.LocalAddr())

	t.mut.Lock()
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
		if err == context.Canceled {
			return nil
		} else if err != nil {
			l.Infoln("Listen (BEP/quic): Accepting connection:", err)

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

		l.Debugln("connect from", session.RemoteAddr())

		streamCtx, cancel := context.WithTimeout(ctx, quicOperationTimeout)
		stream, err := session.AcceptStream(streamCtx)
		cancel()
		if err != nil {
			l.Debugf("failed to accept stream from %s: %v", session.RemoteAddr(), err)
			_ = session.CloseWithError(1, err.Error())
			continue
		}

		t.conns <- newInternalConn(&quicTlsConn{session, stream, nil}, connTypeQUICServer, quicPriority)
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
		factory: f,
	}
	l.ServiceWithError = svcutil.AsService(l.serve, l.String())
	l.nat.Store(stun.NATUnknown)
	return l
}

func (quicListenerFactory) Enabled(cfg config.Configuration) bool {
	return true
}

// quicWrapper provides methods used by quic
// https://github.com/lucas-clemente/quic-go/blob/master/packet_handler_map.go#L85
type quicWrapper struct {
	net.PacketConn
	underlying *net.UDPConn
}

// SetReadBuffer is required by QUIC
func (s *quicWrapper) SetReadBuffer(size int) error {
	return s.underlying.SetReadBuffer(size)
}

// SyscallConn is required by QUIC
func (s *quicWrapper) SyscallConn() (syscall.RawConn, error) {
	return s.underlying.SyscallConn()
}

// oobConn is used to assert that stun package returned a net.PacketConn that implements this interface.
// If it does, we then wrap quicWrapper in oobConnWrapper, to expose those methods to QUIC package.
type oobConn interface {
	ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error)
	WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error)
}

// See: https://pkg.go.dev/github.com/lucas-clemente/quic-go#OOBCapablePacketConn
type oobConnWrapper struct {
	quicWrapper
	oobConn
}
