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

	"github.com/AudriusButkevicius/pfilter"
	"github.com/ccding/go-stun/stun"
	"github.com/lucas-clemente/quic-go"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/nat"
)

const stunRetryInterval = 5 * time.Minute

func init() {
	factory := &quicListenerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		listeners[scheme] = factory
	}
}

type quicListener struct {
	onAddressesChangedNotifier

	uri     *url.URL
	cfg     config.Wrapper
	tlsCfg  *tls.Config
	stop    chan struct{}
	conns   chan internalConn
	factory listenerFactory
	nat     atomic.Value

	address *url.URL
	err     error
	mut     sync.RWMutex
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
	writeTrackingConn := &writeTrackingPacketConn{PacketConn: packetConn}
	packetConn = writeTrackingConn
	filterConn := pfilter.NewPacketFilter(packetConn)
	quicConn := filterConn.NewConn(quicFilterPriority, nil)
	stunConn := filterConn.NewConn(stunFilterPriority, &stunFilter{
		ids: make(map[string]time.Time),
	})

	filterConn.Start()
	registry.Register(t.uri.Scheme, quicConn.(net.PacketConn))

	listener, err := quic.Listen(quicConn, t.tlsCfg, quicConfig)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Infoln("Listen (BEP/quic):", err)
		return
	}

	defer listener.Close()
	defer stunConn.Close()
	defer quicConn.Close()
	defer registry.Unregister(t.uri.Scheme, quicConn.(net.PacketConn))
	defer filterConn.Close()
	defer packetConn.Close()

	l.Infof("QUIC listener (%v) starting", quicConn.LocalAddr())
	defer l.Infof("QUIC listener (%v) shutting down", quicConn.LocalAddr())

	go t.stunRenewal(stunConn, writeTrackingConn.GetLastWrite)

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
	t.mut.RLock()
	if t.address != nil {
		uris = append(uris, t.address)
	}
	t.mut.RUnlock()
	return uris
}

func (t *quicListener) LANAddresses() []*url.URL {
	return []*url.URL{t.uri}
}

func (t *quicListener) Error() error {
	t.mut.RLock()
	err := t.err
	t.mut.RUnlock()
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

func (t *quicListener) stunRenewal(listener net.PacketConn, getLastWriteTime func() time.Time) {
	for {
	disabled:
		if t.stunDisabled() {
			select {
			case <-t.stop:
				return
			case <-time.After(time.Second):
				continue
			}
		}

		l.Debugf("Starting stun for %s", t.uri)

		t.nat.Store(stun.NATUnknown)
		t.mut.Lock()
		t.address = nil
		t.mut.Unlock()

		for _, addr := range t.cfg.StunServers() {
			// This blocks until we hit an exit condition or there are issues with the STUN server.
			// This returns a boolean signifying if a different STUN server should be tried (oppose to the whole thing
			// shutting down and this winding itself down.
			if !t.runStunForServer(listener, addr, getLastWriteTime) {
				// Check exit conditions.

				// Have we been asked to stop?
				select {
				case <-t.stop:
					return
				default:
				}

				// Are we disabled?
				if t.stunDisabled() {
					l.Infoln("STUN disabled")
					goto disabled
				}

				lastNatType := t.nat.Load().(stun.NATType)
				// Unpunchable NAT? Chillout for some time.
				if !isPunchable(lastNatType) {
					break
				}
			}
		}

		// We failed to contact all provided stun servers or the nat is not punchable.
		// Chillout for a while.
		time.Sleep(stunRetryInterval)
	}
}

func (t *quicListener) stunDisabled() bool {
	opts := t.cfg.Options()
	return opts.StunKeepaliveMinS < 1 || opts.StunKeepaliveStartS < 1 || !opts.NATEnabled
}

func (t *quicListener) runStunForServer(listener net.PacketConn, addr string, getLastWriteTime func() time.Time) (tryNext bool) {
	l.Debugf("Running stun for %s via %s", t.uri, addr)

	// Resolve the address, so that in case the server advertises two
	// IPs, we always hit the same one, as otherwise, the mapping might
	// expire as we hit the other address, and cause us to flip flop
	// between servers/external addresses, as a result flooding discovery
	// servers.
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		l.Debugf("%s stun addr resolution on %s: %s", t.uri, addr, err)
		return true
	}
	client := stun.NewClientWithConnection(listener)
	client.SetServerAddr(udpAddr.String())
	client.SetSoftwareName("") // Explicitly unset this, seems to freak some servers out.

	natType, extAddr, err := client.Discover()
	if err != nil || extAddr == nil {
		l.Debugf("%s stun discovery on %s: %s", t.uri, addr, err)
		return true
	}

	// The stun server is most likely borked, try another one.
	if natType == stun.NATError || natType == stun.NATUnknown || natType == stun.NATBlocked {
		l.Debugf("%s stun discovery on %s resolved to %s", t.uri, addr, natType)
		return true
	}

	oldNatType := t.nat.Load().(stun.NATType)
	if oldNatType != natType {
		l.Infof("%s detected NAT type: %s", t.uri, natType)
	}
	l.Debugf("%s detected NAT type: %s", t.uri, natType)
	t.nat.Store(natType)

	// We can't punch through this one, so no point doing keepalives
	// and such, just let the caller check the nat type and work it out themselves.
	if !isPunchable(natType) {
		l.Debugf("%s cannot punch %s, skipping", t.uri, natType)
		return false
	}

	return t.stunKeepAlive(client, addr, extAddr, getLastWriteTime)
}

func (t *quicListener) stunKeepAlive(client *stun.Client, addr string, extAddr *stun.Host, getLastWriteTime func() time.Time) (tryNext bool) {
	var err error
	nextSleep := time.Duration(t.cfg.Options().StunKeepaliveStartS) * time.Second

	l.Debugf("%s starting stun keepalive via %s, next sleep %s", t.uri, addr, nextSleep)

	for {
		changed := false

		uri := *t.uri
		uri.Host = extAddr.TransportAddr()

		t.mut.Lock()

		if t.address == nil || t.address.String() != uri.String() {
			l.Infof("%s resolved external address %s (via %s)", t.uri, uri.String(), addr)
			// If the port has changed (addresses are not equal but the hosts are equal),
			// we're probably spending too much time between keepalives, reduce the sleep.
			if t.address != nil && t.address.Host == uri.Host {
				nextSleep /= 2
				l.Debugf("%s stun port change, next sleep %s", t.uri, nextSleep)
			}
			t.address = &uri
			changed = true
		}
		t.mut.Unlock()

		// This will most likely result in a call to WANAddresses() which tries to
		// get t.mut, so notify while unlocked.
		if changed {
			t.notifyAddressesChanged(t)

			// The stun server is probably stuffed, we've gone beyond min timeout, yet the address keeps changing.
			minSleep := time.Duration(t.cfg.Options().StunKeepaliveMinS) * time.Second
			if nextSleep < minSleep {
				l.Debugf("%s keepalive aborting, sleep below min: %s < %s", t.uri, nextSleep, minSleep)
				return true
			}
		}

		// Adjust the keepalives to fire only nextSleep after last write.
		lastWrite := getLastWriteTime()
		minSleep := time.Duration(t.cfg.Options().StunKeepaliveMinS) * time.Second
		if nextSleep < minSleep {
			nextSleep = minSleep
		}
	tryLater:
		sleepFor := nextSleep
		now := time.Now()

		nextKeepalive := lastWrite.Add(sleepFor)
		if nextKeepalive.After(now) {
			sleepFor = nextKeepalive.Sub(time.Now())
		}

		l.Debugf("%s stun sleeping for %s", t.uri, sleepFor)

		select {
		case <-time.After(sleepFor):
		case <-t.stop:
			l.Debugf("%s stopping, aborting stun", t.uri)
			return false
		}

		if t.stunDisabled() {
			// Disabled, give up
			l.Debugf("%s disabled, aborting stun ", t.uri)
			return false
		}

		// Check if any writes happened while we were sleeping, if they did, sleep again
		lastWrite = getLastWriteTime()
		if gap := time.Now().Sub(lastWrite); gap < nextSleep {
			l.Debugf("%s stun last write gap less than next sleep: %s < %s. Will try later", t.uri, gap, nextSleep)
			goto tryLater
		}

		l.Debugf("%s stun keepalive", t.uri)

		extAddr, err = client.Keepalive()
		if err != nil {
			l.Debugf("%s stun keepalive on %s: %s (%v)", t.uri, addr, err, extAddr)
			return true
		}
	}
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

func isPunchable(natType stun.NATType) bool {
	return natType == stun.NATNone || natType == stun.NATPortRestricted || natType == stun.NATRestricted || natType == stun.NATFull
}
