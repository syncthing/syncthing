// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/juju/ratelimit"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/upnp"
	"github.com/syncthing/syncthing/lib/util"

	"github.com/thejerf/suture"
)

type DialerFactory func(*url.URL, *tls.Config) (*tls.Conn, error)
type ListenerFactory func(*url.URL, *tls.Config, chan<- model.IntermediateConnection)

var (
	dialers                        = make(map[string]DialerFactory, 0)
	listeners                      = make(map[string]ListenerFactory, 0)
	errIncompatibleProtocolVersion = fmt.Errorf("incompatible protocol version")
)

type Model interface {
	protocol.Model
	AddConnection(conn model.Connection, hello protocol.HelloMessage)
	ConnectedTo(remoteID protocol.DeviceID) bool
	IsPaused(remoteID protocol.DeviceID) bool
	OnHello(protocol.DeviceID, net.Addr, protocol.HelloMessage)
	GetHello(protocol.DeviceID) protocol.HelloMessage
}

// Service listens on TLS and dials configured unconnected devices. Successful
// connections are handed to the model.
type Service struct {
	*suture.Supervisor
	cfg                  *config.Wrapper
	myID                 protocol.DeviceID
	model                Model
	tlsCfg               *tls.Config
	discoverer           discover.Finder
	conns                chan model.IntermediateConnection
	upnpService          *upnp.Service
	relayService         relay.Service
	bepProtocolName      string
	tlsDefaultCommonName string
	lans                 []*net.IPNet
	writeRateLimit       *ratelimit.Bucket
	readRateLimit        *ratelimit.Bucket

	lastRelayCheck map[protocol.DeviceID]time.Time

	mut           sync.RWMutex
	connType      map[protocol.DeviceID]model.ConnectionType
	relaysEnabled bool
}

func NewConnectionService(cfg *config.Wrapper, myID protocol.DeviceID, mdl Model, tlsCfg *tls.Config, discoverer discover.Finder, upnpService *upnp.Service,
	relayService relay.Service, bepProtocolName string, tlsDefaultCommonName string, lans []*net.IPNet) *Service {
	service := &Service{
		Supervisor:           suture.NewSimple("connections.Service"),
		cfg:                  cfg,
		myID:                 myID,
		model:                mdl,
		tlsCfg:               tlsCfg,
		discoverer:           discoverer,
		upnpService:          upnpService,
		relayService:         relayService,
		conns:                make(chan model.IntermediateConnection),
		bepProtocolName:      bepProtocolName,
		tlsDefaultCommonName: tlsDefaultCommonName,
		lans:                 lans,

		connType:       make(map[protocol.DeviceID]model.ConnectionType),
		relaysEnabled:  cfg.Options().RelaysEnabled,
		lastRelayCheck: make(map[protocol.DeviceID]time.Time),
	}
	cfg.Subscribe(service)

	// The rate variables are in KiB/s in the UI (despite the camel casing
	// of the name). We multiply by 1024 here to get B/s.
	if service.cfg.Options().MaxSendKbps > 0 {
		service.writeRateLimit = ratelimit.NewBucketWithRate(float64(1024*service.cfg.Options().MaxSendKbps), int64(5*1024*service.cfg.Options().MaxSendKbps))
	}
	if service.cfg.Options().MaxRecvKbps > 0 {
		service.readRateLimit = ratelimit.NewBucketWithRate(float64(1024*service.cfg.Options().MaxRecvKbps), int64(5*1024*service.cfg.Options().MaxRecvKbps))
	}

	// There are several moving parts here; one routine per listening address
	// to handle incoming connections, one routine to periodically attempt
	// outgoing connections, one routine to the the common handling
	// regardless of whether the connection was incoming or outgoing.
	// Furthermore, a relay service which handles incoming requests to connect
	// via the relays.
	//
	// TODO: Clean shutdown, and/or handling config changes on the fly. We
	// partly do this now - new devices and addresses will be picked up, but
	// not new listen addresses and we don't support disconnecting devices
	// that are removed and so on...

	service.Add(serviceFunc(service.connect))
	for _, addr := range service.cfg.Options().ListenAddress {
		uri, err := url.Parse(addr)
		if err != nil {
			l.Infoln("Failed to parse listen address:", addr, err)
			continue
		}

		listener, ok := listeners[uri.Scheme]
		if !ok {
			l.Infoln("Unknown listen address scheme:", uri.String())
			continue
		}

		l.Debugln("listening on", uri)

		service.Add(serviceFunc(func() {
			listener(uri, service.tlsCfg, service.conns)
		}))
	}
	service.Add(serviceFunc(service.handle))

	if service.relayService != nil {
		service.Add(serviceFunc(service.acceptRelayConns))
	}

	return service
}

func (s *Service) handle() {
next:
	for c := range s.conns {
		cs := c.ConnectionState()

		// We should have negotiated the next level protocol "bep/1.0" as part
		// of the TLS handshake. Unfortunately this can't be a hard error,
		// because there are implementations out there that don't support
		// protocol negotiation (iOS for one...).
		if !cs.NegotiatedProtocolIsMutual || cs.NegotiatedProtocol != s.bepProtocolName {
			l.Infof("Peer %s did not negotiate bep/1.0", c.RemoteAddr())
		}

		// We should have received exactly one certificate from the other
		// side. If we didn't, they don't have a device ID and we drop the
		// connection.
		certs := cs.PeerCertificates
		if cl := len(certs); cl != 1 {
			l.Infof("Got peer certificate list of length %d != 1 from %s; protocol error", cl, c.RemoteAddr())
			c.Close()
			continue
		}
		remoteCert := certs[0]
		remoteID := protocol.NewDeviceID(remoteCert.Raw)

		// The device ID should not be that of ourselves. It can happen
		// though, especially in the presence of NAT hairpinning, multiple
		// clients between the same NAT gateway, and global discovery.
		if remoteID == s.myID {
			l.Infof("Connected to myself (%s) - should not happen", remoteID)
			c.Close()
			continue
		}

		hello, err := exchangeHello(c, s.model.GetHello(remoteID))
		if err != nil {
			if err == errIncompatibleProtocolVersion {
				l.Warnf("Connection from %s (%s) with an incompatible version of Syncthing", remoteID, c.RemoteAddr())
			} else {
				l.Infof("Failed to exchange Hello messages with %s (%s): %s", remoteID, c.RemoteAddr(), err)
			}
			c.Close()
			continue next
		}

		s.model.OnHello(remoteID, c.RemoteAddr(), hello)

		// If we have a relay connection, and the new incoming connection is
		// not a relay connection, we should drop that, and prefer the this one.
		s.mut.RLock()
		ct, ok := s.connType[remoteID]
		s.mut.RUnlock()
		if ok && !ct.IsDirect() && c.Type.IsDirect() {
			l.Debugln("Switching connections", remoteID)
			s.model.Close(remoteID, fmt.Errorf("switching connections"))
		} else if s.model.ConnectedTo(remoteID) {
			// We should not already be connected to the other party. TODO: This
			// could use some better handling. If the old connection is dead but
			// hasn't timed out yet we may want to drop *that* connection and keep
			// this one. But in case we are two devices connecting to each other
			// in parallel we don't want to do that or we end up with no
			// connections still established...
			l.Infof("Connected to already connected device (%s)", remoteID)
			c.Close()
			continue
		} else if s.model.IsPaused(remoteID) {
			l.Infof("Connection from paused device (%s)", remoteID)
			c.Close()
			continue
		}

		for deviceID, deviceCfg := range s.cfg.Devices() {
			if deviceID == remoteID {
				// Verify the name on the certificate. By default we set it to
				// "syncthing" when generating, but the user may have replaced
				// the certificate and used another name.
				certName := deviceCfg.CertName
				if certName == "" {
					certName = s.tlsDefaultCommonName
				}
				err := remoteCert.VerifyHostname(certName)
				if err != nil {
					// Incorrect certificate name is something the user most
					// likely wants to know about, since it's an advanced
					// config. Warn instead of Info.
					l.Warnf("Bad certificate from %s (%v): %v", remoteID, c.RemoteAddr(), err)
					c.Close()
					continue next
				}

				// If rate limiting is set, and based on the address we should
				// limit the connection, then we wrap it in a limiter.

				limit := s.shouldLimit(c.RemoteAddr())

				wr := io.Writer(c.Conn)
				if limit && s.writeRateLimit != nil {
					wr = NewWriteLimiter(c.Conn, s.writeRateLimit)
				}

				rd := io.Reader(c.Conn)
				if limit && s.readRateLimit != nil {
					rd = NewReadLimiter(c.Conn, s.readRateLimit)
				}

				name := fmt.Sprintf("%s-%s (%s)", c.LocalAddr(), c.RemoteAddr(), c.Type)
				protoConn := protocol.NewConnection(remoteID, rd, wr, s.model, name, deviceCfg.Compression)

				l.Infof("Established secure connection to %s at %s", remoteID, name)
				l.Debugf("cipher suite: %04X in lan: %t", c.ConnectionState().CipherSuite, !limit)

				s.model.AddConnection(model.Connection{
					c,
					protoConn,
					c.Type,
				}, hello)
				s.mut.Lock()
				s.connType[remoteID] = c.Type
				s.mut.Unlock()
				continue next
			}
		}

		l.Infof("Connection from %s (%s) with ignored device ID %s", c.RemoteAddr(), c.Type, remoteID)
		c.Close()
	}
}

func (s *Service) connect() {
	delay := time.Second
	for {
		l.Debugln("Reconnect loop")
	nextDevice:
		for deviceID, deviceCfg := range s.cfg.Devices() {
			if deviceID == s.myID {
				continue
			}

			l.Debugln("Reconnect loop for", deviceID)

			if s.model.IsPaused(deviceID) {
				continue
			}

			connected := s.model.ConnectedTo(deviceID)

			s.mut.RLock()
			ct, ok := s.connType[deviceID]
			relaysEnabled := s.relaysEnabled
			s.mut.RUnlock()
			if connected && ok && ct.IsDirect() {
				l.Debugln("Already connected to", deviceID, "via", ct.String())
				continue
			}

			addrs, relays := s.resolveAddresses(deviceID, deviceCfg.Addresses)

			for _, addr := range addrs {
				if conn := s.connectDirect(deviceID, addr); conn != nil {
					l.Debugln("Connecting to", deviceID, "via", addr, "succeeded")
					if connected {
						s.model.Close(deviceID, fmt.Errorf("switching connections"))
					}
					s.conns <- model.IntermediateConnection{
						conn, model.ConnectionTypeDirectDial,
					}
					continue nextDevice
				}
				l.Debugln("Connecting to", deviceID, "via", addr, "failed")
			}

			// Only connect via relays if not already connected
			// Also, do not set lastRelayCheck time if we have no relays,
			// as otherwise when we do discover relays, we might have to
			// wait up to RelayReconnectIntervalM to connect again.
			// Also, do not try relays if we are explicitly told not to.
			if connected || len(relays) == 0 || !relaysEnabled {
				l.Debugln("Not connecting via relay", connected, len(relays) == 0, !relaysEnabled)
				continue nextDevice
			}

			reconIntv := time.Duration(s.cfg.Options().RelayReconnectIntervalM) * time.Minute
			if last, ok := s.lastRelayCheck[deviceID]; ok && time.Since(last) < reconIntv {
				l.Debugln("Skipping connecting via relay to", deviceID, "last checked at", last)
				continue nextDevice
			}

			l.Debugln("Trying relay connections to", deviceID, relays)

			s.lastRelayCheck[deviceID] = time.Now()

			for _, addr := range relays {
				if conn := s.connectViaRelay(deviceID, addr); conn != nil {
					l.Debugln("Connecting to", deviceID, "via", addr, "succeeded")
					s.conns <- model.IntermediateConnection{
						conn, model.ConnectionTypeRelayDial,
					}
					continue nextDevice
				}
				l.Debugln("Connecting to", deviceID, "via", addr, "failed")
			}
		}

		time.Sleep(delay)
		delay *= 2
		if maxD := time.Duration(s.cfg.Options().ReconnectIntervalS) * time.Second; delay > maxD {
			delay = maxD
		}
	}
}

func (s *Service) resolveAddresses(deviceID protocol.DeviceID, inAddrs []string) (addrs []string, relays []discover.Relay) {
	for _, addr := range inAddrs {
		if addr == "dynamic" {
			if s.discoverer != nil {
				if t, r, err := s.discoverer.Lookup(deviceID); err == nil {
					addrs = append(addrs, t...)
					relays = append(relays, r...)
				}
			}
		} else {
			addrs = append(addrs, addr)
		}
	}
	return
}

func (s *Service) connectDirect(deviceID protocol.DeviceID, addr string) *tls.Conn {
	uri, err := url.Parse(addr)
	if err != nil {
		l.Infoln("Failed to parse connection url:", addr, err)
		return nil
	}

	dialer, ok := dialers[uri.Scheme]
	if !ok {
		l.Infoln("Unknown address schema", uri)
		return nil
	}

	l.Debugln("dial", deviceID, uri)
	conn, err := dialer(uri, s.tlsCfg)
	if err != nil {
		l.Debugln("dial failed", deviceID, uri, err)
		return nil
	}

	return conn
}

func (s *Service) connectViaRelay(deviceID protocol.DeviceID, addr discover.Relay) *tls.Conn {
	uri, err := url.Parse(addr.URL)
	if err != nil {
		l.Infoln("Failed to parse relay connection url:", addr, err)
		return nil
	}

	inv, err := client.GetInvitationFromRelay(uri, deviceID, s.tlsCfg.Certificates, 10*time.Second)
	if err != nil {
		l.Debugf("Failed to get invitation for %s from %s: %v", deviceID, uri, err)
		return nil
	}
	l.Debugln("Succesfully retrieved relay invitation", inv, "from", uri)

	conn, err := client.JoinSession(inv)
	if err != nil {
		l.Debugf("Failed to join relay session %s: %v", inv, err)
		return nil
	}
	l.Debugln("Successfully joined relay session", inv)

	var tc *tls.Conn

	if inv.ServerSocket {
		tc = tls.Server(conn, s.tlsCfg)
	} else {
		tc = tls.Client(conn, s.tlsCfg)
	}

	err = tc.Handshake()
	if err != nil {
		l.Infof("TLS handshake (BEP/relay %s): %v", inv, err)
		tc.Close()
		return nil
	}

	return tc
}

func (s *Service) acceptRelayConns() {
	for {
		conn := s.relayService.Accept()
		s.conns <- model.IntermediateConnection{
			Conn: conn,
			Type: model.ConnectionTypeRelayAccept,
		}
	}
}

func (s *Service) shouldLimit(addr net.Addr) bool {
	if s.cfg.Options().LimitBandwidthInLan {
		return true
	}

	tcpaddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return true
	}
	for _, lan := range s.lans {
		if lan.Contains(tcpaddr.IP) {
			return false
		}
	}
	return !tcpaddr.IP.IsLoopback()
}

func (s *Service) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (s *Service) CommitConfiguration(from, to config.Configuration) bool {
	s.mut.Lock()
	s.relaysEnabled = to.Options.RelaysEnabled
	s.mut.Unlock()

	// We require a restart if a device as been removed.

	newDevices := make(map[protocol.DeviceID]bool, len(to.Devices))
	for _, dev := range to.Devices {
		newDevices[dev.DeviceID] = true
	}

	for _, dev := range from.Devices {
		if !newDevices[dev.DeviceID] {
			return false
		}
	}

	return true
}

// ExternalAddresses returns a list of addresses that are our best guess for
// where we are reachable from the outside. As a special case, we may return
// one or more addresses with an empty IP address (0.0.0.0 or ::) and just
// port number - this means that the outside address of a NAT gateway should
// be substituted.
func (s *Service) ExternalAddresses() []string {
	return s.addresses(false)
}

// AllAddresses returns a list of addresses that are our best guess for where
// we are reachable from the local network. Same conditions as
// ExternalAddresses, but private IPv4 addresses are included.
func (s *Service) AllAddresses() []string {
	return s.addresses(true)
}

func (s *Service) addresses(includePrivateIPV4 bool) []string {
	var addrs []string

	// Grab our listen addresses from the config. Unspecified ones are passed
	// on verbatim (to be interpreted by a global discovery server or local
	// discovery peer). Public addresses are passed on verbatim. Private
	// addresses are filtered.
	for _, addrStr := range s.cfg.Options().ListenAddress {
		addrURL, err := url.Parse(addrStr)
		if err != nil {
			l.Infoln("Listen address", addrStr, "is invalid:", err)
			continue
		}
		addr, err := net.ResolveTCPAddr(addrURL.Scheme, addrURL.Host)
		if err != nil {
			l.Infoln("Listen address", addrStr, "is invalid:", err)
			continue
		}

		if addr.IP == nil || addr.IP.IsUnspecified() {
			// Address like 0.0.0.0:22000 or [::]:22000 or :22000; include as is.
			addrs = append(addrs, util.Address(addrURL.Scheme, addr.String()))
		} else if isPublicIPv4(addr.IP) || isPublicIPv6(addr.IP) {
			// A public address; include as is.
			addrs = append(addrs, util.Address(addrURL.Scheme, addr.String()))
		} else if includePrivateIPV4 && addr.IP.To4().IsGlobalUnicast() {
			// A private IPv4 address.
			addrs = append(addrs, util.Address(addrURL.Scheme, addr.String()))
		}
	}

	// Get an external port mapping from the upnpService, if it has one. If so,
	// add it as another unspecified address.
	if s.upnpService != nil {
		if port := s.upnpService.ExternalPort(); port != 0 {
			addrs = append(addrs, fmt.Sprintf("tcp://:%d", port))
		}
	}

	return addrs
}

func isPublicIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		// Not an IPv4 address (IPv6)
		return false
	}

	// IsGlobalUnicast below only checks that it's not link local or
	// multicast, and we want to exclude private (NAT:ed) addresses as well.
	rfc1918 := []net.IPNet{
		{IP: net.IP{10, 0, 0, 0}, Mask: net.IPMask{255, 0, 0, 0}},
		{IP: net.IP{172, 16, 0, 0}, Mask: net.IPMask{255, 240, 0, 0}},
		{IP: net.IP{192, 168, 0, 0}, Mask: net.IPMask{255, 255, 0, 0}},
	}
	for _, n := range rfc1918 {
		if n.Contains(ip) {
			return false
		}
	}

	return ip.IsGlobalUnicast()
}

func isPublicIPv6(ip net.IP) bool {
	if ip.To4() != nil {
		// Not an IPv6 address (IPv4)
		// (To16() returns a v6 mapped v4 address so can't be used to check
		// that it's an actual v6 address)
		return false
	}

	return ip.IsGlobalUnicast()
}

func exchangeHello(c net.Conn, h protocol.HelloMessage) (protocol.HelloMessage, error) {
	if err := c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return protocol.HelloMessage{}, err
	}
	defer c.SetDeadline(time.Time{})

	buf := make([]byte, protocol.HelloMessageMaxSize)
	copy(buf, h.MustMarshalXDR())

	if _, err := c.Write(buf); err != nil {
		return protocol.HelloMessage{}, err
	}

	var hello protocol.HelloMessage

	if _, err := io.ReadFull(c, buf); err != nil {
		return protocol.HelloMessage{}, err
	}

	if err := hello.UnmarshalXDR(buf); err != nil {
		return protocol.HelloMessage{}, err
	}

	if hello.ProtocolVersion != protocol.Version {
		return protocol.HelloMessage{}, errIncompatibleProtocolVersion
	}

	return hello, nil
}

// serviceFunc wraps a function to create a suture.Service without stop
// functionality.
type serviceFunc func()

func (f serviceFunc) Serve() { f() }
func (f serviceFunc) Stop()  {}
