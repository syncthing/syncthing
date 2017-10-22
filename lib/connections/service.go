// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"

	// Registers NAT service providers
	_ "github.com/syncthing/syncthing/lib/pmp"
	_ "github.com/syncthing/syncthing/lib/upnp"

	"github.com/thejerf/suture"
	"golang.org/x/time/rate"
)

var (
	dialers   = make(map[string]dialerFactory, 0)
	listeners = make(map[string]listenerFactory, 0)
)

const (
	perDeviceWarningIntv = 15 * time.Minute
	tlsHandshakeTimeout  = 10 * time.Second
)

// From go/src/crypto/tls/cipher_suites.go
var tlsCipherSuiteNames = map[uint16]string{
	0x0005: "TLS_RSA_WITH_RC4_128_SHA",
	0x000a: "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
	0x002f: "TLS_RSA_WITH_AES_128_CBC_SHA",
	0x0035: "TLS_RSA_WITH_AES_256_CBC_SHA",
	0x003c: "TLS_RSA_WITH_AES_128_CBC_SHA256",
	0x009c: "TLS_RSA_WITH_AES_128_GCM_SHA256",
	0x009d: "TLS_RSA_WITH_AES_256_GCM_SHA384",
	0xc007: "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA",
	0xc009: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
	0xc00a: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
	0xc011: "TLS_ECDHE_RSA_WITH_RC4_128_SHA",
	0xc012: "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA",
	0xc013: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
	0xc014: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
	0xc023: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
	0xc027: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
	0xc02f: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	0xc02b: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	0xc030: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	0xc02c: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	0xcca8: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
	0xcca9: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
}

// Service listens and dials all configured unconnected devices, via supported
// dialers. Successful connections are handed to the model.
type Service struct {
	*suture.Supervisor
	cfg                  *config.Wrapper
	myID                 protocol.DeviceID
	model                Model
	tlsCfg               *tls.Config
	discoverer           discover.Finder
	conns                chan internalConn
	bepProtocolName      string
	tlsDefaultCommonName string
	lans                 []*net.IPNet
	limiter              *limiter
	natService           *nat.Service
	natServiceToken      *suture.ServiceToken

	listenersMut       sync.RWMutex
	listeners          map[string]genericListener
	listenerTokens     map[string]suture.ServiceToken
	listenerSupervisor *suture.Supervisor

	curConMut         sync.Mutex
	currentConnection map[protocol.DeviceID]completeConn
}

func NewService(cfg *config.Wrapper, myID protocol.DeviceID, mdl Model, tlsCfg *tls.Config, discoverer discover.Finder,
	bepProtocolName string, tlsDefaultCommonName string, lans []*net.IPNet) *Service {

	service := &Service{
		Supervisor: suture.New("connections.Service", suture.Spec{
			Log: func(line string) {
				l.Infoln(line)
			},
		}),
		cfg:                  cfg,
		myID:                 myID,
		model:                mdl,
		tlsCfg:               tlsCfg,
		discoverer:           discoverer,
		conns:                make(chan internalConn),
		bepProtocolName:      bepProtocolName,
		tlsDefaultCommonName: tlsDefaultCommonName,
		lans:                 lans,
		limiter:              newLimiter(cfg),
		natService:           nat.NewService(myID, cfg),

		listenersMut:   sync.NewRWMutex(),
		listeners:      make(map[string]genericListener),
		listenerTokens: make(map[string]suture.ServiceToken),

		// A listener can fail twice, rapidly. Any more than that and it
		// will be put on suspension for ten minutes. Restarts and changes
		// due to config are done by removing and adding services, so are
		// not subject to these limitations.
		listenerSupervisor: suture.New("c.S.listenerSupervisor", suture.Spec{
			Log: func(line string) {
				l.Infoln(line)
			},
			FailureThreshold: 2,
			FailureBackoff:   600 * time.Second,
		}),

		curConMut:         sync.NewMutex(),
		currentConnection: make(map[protocol.DeviceID]completeConn),
	}
	cfg.Subscribe(service)

	raw := cfg.RawCopy()
	// Actually starts the listeners and NAT service
	// Need to start this before service.connect so that any dials that
	// try punch through already have a listener to cling on.
	service.CommitConfiguration(raw, raw)

	// There are several moving parts here; one routine per listening address
	// (handled in configuration changing) to handle incoming connections,
	// one routine to periodically attempt outgoing connections, one routine to
	// the common handling regardless of whether the connection was
	// incoming or outgoing.

	service.Add(serviceFunc(service.connect))
	service.Add(serviceFunc(service.handle))
	service.Add(service.listenerSupervisor)

	return service
}

var (
	errDisabled = errors.New("disabled by configuration")
)

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

		c.SetDeadline(time.Now().Add(20 * time.Second))
		hello, err := protocol.ExchangeHello(c, s.model.GetHello(remoteID))
		if err != nil {
			if protocol.IsVersionMismatch(err) {
				// The error will be a relatively user friendly description
				// of what's wrong with the version compatibility. By
				// default identify the other side by device ID and IP.
				remote := fmt.Sprintf("%v (%v)", remoteID, c.RemoteAddr())
				if hello.DeviceName != "" {
					// If the name was set in the hello return, use that to
					// give the user more info about which device is the
					// affected one. It probably says more than the remote
					// IP.
					remote = fmt.Sprintf("%q (%s %s, %v)", hello.DeviceName, hello.ClientName, hello.ClientVersion, remoteID)
				}
				msg := fmt.Sprintf("Connecting to %s: %s", remote, err)
				warningFor(remoteID, msg)
			} else {
				// It's something else - connection reset or whatever
				l.Infof("Failed to exchange Hello messages with %s (%s): %s", remoteID, c.RemoteAddr(), err)
			}
			c.Close()
			continue
		}
		c.SetDeadline(time.Time{})

		// The Model will return an error for devices that we don't want to
		// have a connection with for whatever reason, for example unknown devices.
		if err := s.model.OnHello(remoteID, c.RemoteAddr(), hello); err != nil {
			l.Infof("Connection from %s at %s (%s) rejected: %v", remoteID, c.RemoteAddr(), c.Type(), err)
			c.Close()
			continue
		}

		// If we have a relay connection, and the new incoming connection is
		// not a relay connection, we should drop that, and prefer the this one.
		connected := s.model.ConnectedTo(remoteID)
		s.curConMut.Lock()
		ct, ok := s.currentConnection[remoteID]
		s.curConMut.Unlock()
		priorityKnown := ok && connected

		// Lower priority is better, just like nice etc.
		if priorityKnown && ct.internalConn.priority > c.priority {
			l.Debugln("Switching connections", remoteID)
		} else if connected {
			// We should not already be connected to the other party. TODO: This
			// could use some better handling. If the old connection is dead but
			// hasn't timed out yet we may want to drop *that* connection and keep
			// this one. But in case we are two devices connecting to each other
			// in parallel we don't want to do that or we end up with no
			// connections still established...
			l.Infof("Connected to already connected device (%s)", remoteID)
			c.Close()
			continue
		}

		deviceCfg, ok := s.cfg.Device(remoteID)
		if !ok {
			panic("bug: unknown device should already have been rejected")
		}

		// Verify the name on the certificate. By default we set it to
		// "syncthing" when generating, but the user may have replaced
		// the certificate and used another name.
		certName := deviceCfg.CertName
		if certName == "" {
			certName = s.tlsDefaultCommonName
		}
		if err := remoteCert.VerifyHostname(certName); err != nil {
			// Incorrect certificate name is something the user most
			// likely wants to know about, since it's an advanced
			// config. Warn instead of Info.
			l.Warnf("Bad certificate from %s (%v): %v", remoteID, c.RemoteAddr(), err)
			c.Close()
			continue next
		}

		// Wrap the connection in rate limiters. The limiter itself will
		// keep up with config changes to the rate and whether or not LAN
		// connections are limited.
		isLAN := s.isLAN(c.RemoteAddr())
		wr := s.limiter.newWriteLimiter(c, isLAN)
		rd := s.limiter.newReadLimiter(c, isLAN)

		name := fmt.Sprintf("%s-%s (%s)", c.LocalAddr(), c.RemoteAddr(), c.Type())
		protoConn := protocol.NewConnection(remoteID, rd, wr, s.model, name, deviceCfg.Compression)
		modelConn := completeConn{c, protoConn}

		l.Infof("Established secure connection to %s at %s (%s)", remoteID, name, tlsCipherSuiteNames[c.ConnectionState().CipherSuite])

		s.model.AddConnection(modelConn, hello)
		s.curConMut.Lock()
		s.currentConnection[remoteID] = modelConn
		s.curConMut.Unlock()
		continue next
	}
}

func (s *Service) connect() {
	nextDial := make(map[string]time.Time)

	// Used as delay for the first few connection attempts, increases
	// exponentially
	initialRampup := time.Second

	// Calculated from actual dialers reconnectInterval
	var sleep time.Duration

	for {
		cfg := s.cfg.RawCopy()

		bestDialerPrio := 1<<31 - 1 // worse prio won't build on 32 bit
		for _, df := range dialers {
			if !df.Enabled(cfg) {
				continue
			}
			if prio := df.Priority(); prio < bestDialerPrio {
				bestDialerPrio = prio
			}
		}

		l.Debugln("Reconnect loop")

		now := time.Now()
		var seen []string

	nextDevice:
		for _, deviceCfg := range cfg.Devices {
			deviceID := deviceCfg.DeviceID
			if deviceID == s.myID {
				continue
			}

			if deviceCfg.Paused {
				continue
			}

			connected := s.model.ConnectedTo(deviceID)
			s.curConMut.Lock()
			ct, ok := s.currentConnection[deviceID]
			s.curConMut.Unlock()
			priorityKnown := ok && connected

			if priorityKnown && ct.internalConn.priority == bestDialerPrio {
				// Things are already as good as they can get.
				continue
			}

			l.Debugln("Reconnect loop for", deviceID)

			var addrs []string
			for _, addr := range deviceCfg.Addresses {
				if addr == "dynamic" {
					if s.discoverer != nil {
						if t, err := s.discoverer.Lookup(deviceID); err == nil {
							addrs = append(addrs, t...)
						}
					}
				} else {
					addrs = append(addrs, addr)
				}
			}

			seen = append(seen, addrs...)

			for _, addr := range addrs {
				nextDialAt, ok := nextDial[addr]
				if ok && initialRampup >= sleep && nextDialAt.After(now) {
					l.Debugf("Not dialing %v as sleep is %v, next dial is at %s and current time is %s", addr, sleep, nextDialAt, now)
					continue
				}
				// If we fail at any step before actually getting the dialer
				// retry in a minute
				nextDial[addr] = now.Add(time.Minute)

				uri, err := url.Parse(addr)
				if err != nil {
					l.Infof("Dialer for %s: %v", addr, err)
					continue
				}

				if len(deviceCfg.AllowedNetworks) > 0 {
					if !IsAllowedNetwork(uri.Host, deviceCfg.AllowedNetworks) {
						l.Debugln("Network for", uri, "is disallowed")
						continue
					}
				}

				dialerFactory, err := s.getDialerFactory(cfg, uri)
				if err == errDisabled {
					l.Debugln("Dialer for", uri, "is disabled")
					continue
				}
				if err != nil {
					l.Infof("Dialer for %v: %v", uri, err)
					continue
				}

				if priorityKnown && dialerFactory.Priority() >= ct.internalConn.priority {
					l.Debugf("Not dialing using %s as priority is less than current connection (%d >= %d)", dialerFactory, dialerFactory.Priority(), ct.internalConn.priority)
					continue
				}

				dialer := dialerFactory.New(s.cfg, s.tlsCfg)
				l.Debugln("dial", deviceCfg.DeviceID, uri)
				nextDial[addr] = now.Add(dialer.RedialFrequency())

				conn, err := dialer.Dial(deviceID, uri)
				if err != nil {
					l.Debugln("dial failed", deviceCfg.DeviceID, uri, err)
					continue
				}

				s.conns <- conn
				continue nextDevice
			}
		}

		nextDial, sleep = filterAndFindSleepDuration(nextDial, seen, now)

		if initialRampup < sleep {
			l.Debugln("initial rampup; sleep", initialRampup, "and update to", initialRampup*2)
			time.Sleep(initialRampup)
			initialRampup *= 2
		} else {
			l.Debugln("sleep until next dial", sleep)
			time.Sleep(sleep)
		}
	}
}

func (s *Service) isLAN(addr net.Addr) bool {
	tcpaddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return false
	}
	for _, lan := range s.lans {
		if lan.Contains(tcpaddr.IP) {
			return true
		}
	}
	return tcpaddr.IP.IsLoopback()
}

func (s *Service) createListener(factory listenerFactory, uri *url.URL) bool {
	// must be called with listenerMut held

	l.Debugln("Starting listener", uri)

	listener := factory.New(uri, s.cfg, s.tlsCfg, s.conns, s.natService)
	listener.OnAddressesChanged(s.logListenAddressesChangedEvent)
	s.listeners[uri.String()] = listener
	s.listenerTokens[uri.String()] = s.listenerSupervisor.Add(listener)
	return true
}

func (s *Service) logListenAddressesChangedEvent(l genericListener) {
	events.Default.Log(events.ListenAddressesChanged, map[string]interface{}{
		"address": l.URI(),
		"lan":     l.LANAddresses(),
		"wan":     l.WANAddresses(),
	})
}

func (s *Service) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (s *Service) CommitConfiguration(from, to config.Configuration) bool {
	newDevices := make(map[protocol.DeviceID]bool, len(to.Devices))
	for _, dev := range to.Devices {
		newDevices[dev.DeviceID] = true
	}

	for _, dev := range from.Devices {
		if !newDevices[dev.DeviceID] {
			s.curConMut.Lock()
			delete(s.currentConnection, dev.DeviceID)
			s.curConMut.Unlock()
			warningLimitersMut.Lock()
			delete(warningLimiters, dev.DeviceID)
			warningLimitersMut.Unlock()
		}
	}

	s.listenersMut.Lock()
	seen := make(map[string]struct{})
	for _, addr := range config.Wrap("", to).ListenAddresses() {
		if _, ok := s.listeners[addr]; ok {
			seen[addr] = struct{}{}
			continue
		}

		uri, err := url.Parse(addr)
		if err != nil {
			l.Infof("Listener for %s: %v", addr, err)
			continue
		}

		factory, err := s.getListenerFactory(to, uri)
		if err == errDisabled {
			l.Debugln("Listener for", uri, "is disabled")
			continue
		}
		if err != nil {
			l.Infof("Listener for %v: %v", uri, err)
			continue
		}

		s.createListener(factory, uri)
		seen[addr] = struct{}{}
	}

	for addr, listener := range s.listeners {
		if _, ok := seen[addr]; !ok || !listener.Factory().Enabled(to) {
			l.Debugln("Stopping listener", addr)
			s.listenerSupervisor.Remove(s.listenerTokens[addr])
			delete(s.listenerTokens, addr)
			delete(s.listeners, addr)
		}
	}
	s.listenersMut.Unlock()

	if to.Options.NATEnabled && s.natServiceToken == nil {
		l.Debugln("Starting NAT service")
		token := s.Add(s.natService)
		s.natServiceToken = &token
	} else if !to.Options.NATEnabled && s.natServiceToken != nil {
		l.Debugln("Stopping NAT service")
		s.Remove(*s.natServiceToken)
		s.natServiceToken = nil
	}

	return true
}

func (s *Service) AllAddresses() []string {
	s.listenersMut.RLock()
	var addrs []string
	for _, listener := range s.listeners {
		for _, lanAddr := range listener.LANAddresses() {
			addrs = append(addrs, lanAddr.String())
		}
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	s.listenersMut.RUnlock()
	return util.UniqueStrings(addrs)
}

func (s *Service) ExternalAddresses() []string {
	s.listenersMut.RLock()
	var addrs []string
	for _, listener := range s.listeners {
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	s.listenersMut.RUnlock()
	return util.UniqueStrings(addrs)
}

func (s *Service) Status() map[string]interface{} {
	s.listenersMut.RLock()
	result := make(map[string]interface{})
	for addr, listener := range s.listeners {
		status := make(map[string]interface{})

		err := listener.Error()
		if err != nil {
			status["error"] = err.Error()
		}

		status["lanAddresses"] = urlsToStrings(listener.LANAddresses())
		status["wanAddresses"] = urlsToStrings(listener.WANAddresses())

		result[addr] = status
	}
	s.listenersMut.RUnlock()
	return result
}

func (s *Service) NATType() string {
	s.listenersMut.RLock()
	defer s.listenersMut.RUnlock()
	for _, listener := range s.listeners {
		natType := listener.NATType()
		if natType != "unknown" {
			return natType
		}
	}
	return "unknown"
}

func (s *Service) getDialerFactory(cfg config.Configuration, uri *url.URL) (dialerFactory, error) {
	dialerFactory, ok := dialers[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown address scheme %q", uri.Scheme)
	}

	if !dialerFactory.Enabled(cfg) {
		return nil, errDisabled
	}

	return dialerFactory, nil
}

func (s *Service) getListenerFactory(cfg config.Configuration, uri *url.URL) (listenerFactory, error) {
	listenerFactory, ok := listeners[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown address scheme %q", uri.Scheme)
	}

	if !listenerFactory.Enabled(cfg) {
		return nil, errDisabled
	}

	return listenerFactory, nil
}

func filterAndFindSleepDuration(nextDial map[string]time.Time, seen []string, now time.Time) (map[string]time.Time, time.Duration) {
	newNextDial := make(map[string]time.Time)

	for _, addr := range seen {
		nextDialAt, ok := nextDial[addr]
		if ok {
			newNextDial[addr] = nextDialAt
		}
	}

	min := time.Minute
	for _, next := range newNextDial {
		cur := next.Sub(now)
		if cur < min {
			min = cur
		}
	}
	return newNextDial, min
}

func urlsToStrings(urls []*url.URL) []string {
	strings := make([]string, len(urls))
	for i, url := range urls {
		strings[i] = url.String()
	}
	return strings
}

var warningLimiters = make(map[protocol.DeviceID]*rate.Limiter)
var warningLimitersMut = sync.NewMutex()

func warningFor(dev protocol.DeviceID, msg string) {
	warningLimitersMut.Lock()
	defer warningLimitersMut.Unlock()
	lim, ok := warningLimiters[dev]
	if !ok {
		lim = rate.NewLimiter(rate.Every(perDeviceWarningIntv), 1)
		warningLimiters[dev] = lim
	}
	if lim.Allow() {
		l.Warnln(msg)
	}
}

func tlsTimedHandshake(tc *tls.Conn) error {
	tc.SetDeadline(time.Now().Add(tlsHandshakeTimeout))
	defer tc.SetDeadline(time.Time{})
	return tc.Handshake()
}

// IsAllowedNetwork returns true if the given host (IP or resolvable
// hostname) is in the set of allowed networks (CIDR format only).
func IsAllowedNetwork(host string, allowed []string) bool {
	if hostNoPort, _, err := net.SplitHostPort(host); err == nil {
		host = hostNoPort
	}

	addr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return false
	}

	for _, n := range allowed {
		result := true
		if strings.HasPrefix(n, "!") {
			result = false
			n = n[1:]
		}
		_, cidr, err := net.ParseCIDR(n)
		if err != nil {
			continue
		}
		if cidr.Contains(addr.IP) {
			return result
		}
	}

	return false
}
