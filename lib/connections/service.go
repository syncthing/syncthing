// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"

	// Registers NAT service providers
	_ "github.com/syncthing/syncthing/lib/pmp"
	_ "github.com/syncthing/syncthing/lib/upnp"

	"github.com/pkg/errors"
	"github.com/thejerf/suture/v4"
	"golang.org/x/time/rate"
)

var (
	dialers   = make(map[string]dialerFactory)
	listeners = make(map[string]listenerFactory)
)

var (
	// Dialers and listeners return errUnsupported (or a wrapped variant)
	// when they are intentionally out of service due to configuration,
	// build, etc. This is not logged loudly.
	errUnsupported = errors.New("unsupported protocol")

	// These are specific explanations for errUnsupported.
	errDisabled   = fmt.Errorf("%w: disabled by configuration", errUnsupported)
	errDeprecated = fmt.Errorf("%w: deprecated", errUnsupported)
	errNotInBuild = fmt.Errorf("%w: disabled at build time", errUnsupported)
)

const (
	perDeviceWarningIntv    = 15 * time.Minute
	tlsHandshakeTimeout     = 10 * time.Second
	minConnectionReplaceAge = 10 * time.Second
)

// From go/src/crypto/tls/cipher_suites.go
var tlsCipherSuiteNames = map[uint16]string{
	// TLS 1.2
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

	// TLS 1.3
	0x1301: "TLS_AES_128_GCM_SHA256",
	0x1302: "TLS_AES_256_GCM_SHA384",
	0x1303: "TLS_CHACHA20_POLY1305_SHA256",
}

var tlsVersionNames = map[uint16]string{
	tls.VersionTLS12: "TLS1.2",
	tls.VersionTLS13: "TLS1.3",
}

// Service listens and dials all configured unconnected devices, via supported
// dialers. Successful connections are handed to the model.
type Service interface {
	suture.Service
	discover.AddressLister
	ListenerStatus() map[string]ListenerStatusEntry
	ConnectionStatus() map[string]ConnectionStatusEntry
	NATType() string
}

type ListenerStatusEntry struct {
	Error        *string  `json:"error"`
	LANAddresses []string `json:"lanAddresses"`
	WANAddresses []string `json:"wanAddresses"`
}

type ConnectionStatusEntry struct {
	When  time.Time `json:"when"`
	Error *string   `json:"error"`
}

type service struct {
	*suture.Supervisor
	connectionStatusHandler

	cfg                  config.Wrapper
	myID                 protocol.DeviceID
	model                Model
	tlsCfg               *tls.Config
	discoverer           discover.Finder
	conns                chan internalConn
	bepProtocolName      string
	tlsDefaultCommonName string
	limiter              *limiter
	natService           *nat.Service
	evLogger             events.Logger

	listenersMut       sync.RWMutex
	listeners          map[string]genericListener
	listenerTokens     map[string]suture.ServiceToken
	listenerSupervisor *suture.Supervisor
}

func NewService(cfg config.Wrapper, myID protocol.DeviceID, mdl Model, tlsCfg *tls.Config, discoverer discover.Finder, bepProtocolName string, tlsDefaultCommonName string, evLogger events.Logger) Service {
	spec := util.Spec()
	spec.EventHook = func(e suture.Event) {
		l.Infoln(e)
	}
	service := &service{
		Supervisor:              suture.New("connections.Service", spec),
		connectionStatusHandler: newConnectionStatusHandler(),

		cfg:                  cfg,
		myID:                 myID,
		model:                mdl,
		tlsCfg:               tlsCfg,
		discoverer:           discoverer,
		conns:                make(chan internalConn),
		bepProtocolName:      bepProtocolName,
		tlsDefaultCommonName: tlsDefaultCommonName,
		limiter:              newLimiter(myID, cfg),
		natService:           nat.NewService(myID, cfg),
		evLogger:             evLogger,

		listenersMut:   sync.NewRWMutex(),
		listeners:      make(map[string]genericListener),
		listenerTokens: make(map[string]suture.ServiceToken),

		// A listener can fail twice, rapidly. Any more than that and it
		// will be put on suspension for ten minutes. Restarts and changes
		// due to config are done by removing and adding services, so are
		// not subject to these limitations.
		listenerSupervisor: suture.New("c.S.listenerSupervisor", suture.Spec{
			EventHook: func(e suture.Event) {
				l.Infoln(e)
			},
			FailureThreshold:  2,
			FailureBackoff:    600 * time.Second,
			PassThroughPanics: true,
		}),
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

	service.Add(util.AsService(service.connect, fmt.Sprintf("%s/connect", service)))
	service.Add(util.AsService(service.handle, fmt.Sprintf("%s/handle", service)))
	service.Add(service.listenerSupervisor)
	service.Add(service.natService)

	util.OnSupervisorDone(service.Supervisor, func() {
		service.cfg.Unsubscribe(service.limiter)
		service.cfg.Unsubscribe(service)
	})

	return service
}

func (s *service) handle(ctx context.Context) error {
	var c internalConn
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case c = <-s.conns:
		}

		cs := c.ConnectionState()

		// We should have negotiated the next level protocol "bep/1.0" as part
		// of the TLS handshake. Unfortunately this can't be a hard error,
		// because there are implementations out there that don't support
		// protocol negotiation (iOS for one...).
		if !cs.NegotiatedProtocolIsMutual || cs.NegotiatedProtocol != s.bepProtocolName {
			l.Infof("Peer at %s did not negotiate bep/1.0", c)
		}

		// We should have received exactly one certificate from the other
		// side. If we didn't, they don't have a device ID and we drop the
		// connection.
		certs := cs.PeerCertificates
		if cl := len(certs); cl != 1 {
			l.Infof("Got peer certificate list of length %d != 1 from peer at %s; protocol error", cl, c)
			c.Close()
			continue
		}
		remoteCert := certs[0]
		remoteID := protocol.NewDeviceID(remoteCert.Raw)

		// The device ID should not be that of ourselves. It can happen
		// though, especially in the presence of NAT hairpinning, multiple
		// clients between the same NAT gateway, and global discovery.
		if remoteID == s.myID {
			l.Infof("Connected to myself (%s) at %s - should not happen", remoteID, c)
			c.Close()
			continue
		}

		_ = c.SetDeadline(time.Now().Add(20 * time.Second))
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
				l.Infof("Failed to exchange Hello messages with %s at %s: %s", remoteID, c, err)
			}
			c.Close()
			continue
		}
		_ = c.SetDeadline(time.Time{})

		// The Model will return an error for devices that we don't want to
		// have a connection with for whatever reason, for example unknown devices.
		if err := s.model.OnHello(remoteID, c.RemoteAddr(), hello); err != nil {
			l.Infof("Connection from %s at %s (%s) rejected: %v", remoteID, c.RemoteAddr(), c.Type(), err)
			c.Close()
			continue
		}

		// If we have a relay connection, and the new incoming connection is
		// not a relay connection, we should drop that, and prefer this one.
		ct, connected := s.model.Connection(remoteID)

		// Lower priority is better, just like nice etc.
		if connected && (ct.Priority() > c.priority || time.Since(ct.Statistics().StartedAt) > minConnectionReplaceAge) {
			l.Debugf("Switching connections %s (existing: %s new: %s)", remoteID, ct, c)
		} else if connected {
			// We should not already be connected to the other party. TODO: This
			// could use some better handling. If the old connection is dead but
			// hasn't timed out yet we may want to drop *that* connection and keep
			// this one. But in case we are two devices connecting to each other
			// in parallel we don't want to do that or we end up with no
			// connections still established...
			l.Infof("Connected to already connected device %s (existing: %s new: %s)", remoteID, ct, c)
			c.Close()
			continue
		}

		deviceCfg, ok := s.cfg.Device(remoteID)
		if !ok {
			l.Infof("Device %s removed from config during connection attempt at %s", remoteID, c)
			c.Close()
			continue
		}

		// Verify the name on the certificate. By default we set it to
		// "syncthing" when generating, but the user may have replaced
		// the certificate and used another name.
		certName := deviceCfg.CertName
		if certName == "" {
			certName = s.tlsDefaultCommonName
		}
		if remoteCert.Subject.CommonName == certName {
			// All good. We do this check because our old style certificates
			// have "syncthing" in the CommonName field and no SANs, which
			// is not accepted by VerifyHostname() any more as of Go 1.15.
		} else if err := remoteCert.VerifyHostname(certName); err != nil {
			// Incorrect certificate name is something the user most
			// likely wants to know about, since it's an advanced
			// config. Warn instead of Info.
			l.Warnf("Bad certificate from %s at %s: %v", remoteID, c, err)
			c.Close()
			continue
		}

		// Wrap the connection in rate limiters. The limiter itself will
		// keep up with config changes to the rate and whether or not LAN
		// connections are limited.
		isLAN := s.isLAN(c.RemoteAddr())
		rd, wr := s.limiter.getLimiters(remoteID, c, isLAN)

		var protoConn protocol.Connection
		passwords := s.cfg.FolderPasswords(remoteID)
		if len(passwords) > 0 {
			protoConn = protocol.NewEncryptedConnection(passwords, remoteID, rd, wr, c, s.model, c, deviceCfg.Compression)
		} else {
			protoConn = protocol.NewConnection(remoteID, rd, wr, c, s.model, c, deviceCfg.Compression)
		}

		l.Infof("Established secure connection to %s at %s", remoteID, c)

		s.model.AddConnection(protoConn, hello)
		continue
	}
}

func (s *service) connect(ctx context.Context) error {
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
			if df.Valid(cfg) != nil {
				continue
			}
			if prio := df.Priority(); prio < bestDialerPrio {
				bestDialerPrio = prio
			}
		}

		l.Debugln("Reconnect loop")

		now := time.Now()
		var seen []string

		for _, deviceCfg := range cfg.Devices {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			deviceID := deviceCfg.DeviceID
			if deviceID == s.myID {
				continue
			}

			if deviceCfg.Paused {
				continue
			}

			ct, connected := s.model.Connection(deviceID)

			if connected && ct.Priority() == bestDialerPrio {
				// Things are already as good as they can get.
				continue
			}

			var addrs []string
			for _, addr := range deviceCfg.Addresses {
				if addr == "dynamic" {
					if s.discoverer != nil {
						if t, err := s.discoverer.Lookup(ctx, deviceID); err == nil {
							addrs = append(addrs, t...)
						}
					}
				} else {
					addrs = append(addrs, addr)
				}
			}

			addrs = util.UniqueTrimmedStrings(addrs)

			l.Debugln("Reconnect loop for", deviceID, addrs)

			dialTargets := make([]dialTarget, 0)

			for _, addr := range addrs {
				// Use a special key that is more than just the address, as you might have two devices connected to the same relay
				nextDialKey := deviceID.String() + "/" + addr
				seen = append(seen, nextDialKey)
				nextDialAt, ok := nextDial[nextDialKey]
				if ok && initialRampup >= sleep && nextDialAt.After(now) {
					l.Debugf("Not dialing %s via %v as sleep is %v, next dial is at %s and current time is %s", deviceID, addr, sleep, nextDialAt, now)
					continue
				}
				// If we fail at any step before actually getting the dialer
				// retry in a minute
				nextDial[nextDialKey] = now.Add(time.Minute)

				uri, err := url.Parse(addr)
				if err != nil {
					s.setConnectionStatus(addr, err)
					l.Infof("Parsing dialer address %s: %v", addr, err)
					continue
				}

				if len(deviceCfg.AllowedNetworks) > 0 {
					if !IsAllowedNetwork(uri.Host, deviceCfg.AllowedNetworks) {
						s.setConnectionStatus(addr, errors.New("network disallowed"))
						l.Debugln("Network for", uri, "is disallowed")
						continue
					}
				}

				dialerFactory, err := getDialerFactory(cfg, uri)
				if err != nil {
					s.setConnectionStatus(addr, err)
				}
				if errors.Is(err, errUnsupported) {
					l.Debugf("Dialer for %v: %v", uri, err)
					continue
				} else if err != nil {
					l.Infof("Dialer for %v: %v", uri, err)
					continue
				}

				priority := dialerFactory.Priority()

				if connected && priority >= ct.Priority() {
					l.Debugf("Not dialing using %s as priority is less than current connection (%d >= %d)", dialerFactory, dialerFactory.Priority(), ct.Priority())
					continue
				}

				dialer := dialerFactory.New(s.cfg.Options(), s.tlsCfg)
				nextDial[nextDialKey] = now.Add(dialer.RedialFrequency())

				// For LAN addresses, increase the priority so that we
				// try these first.
				switch {
				case dialerFactory.AlwaysWAN():
					// Do nothing.
				case s.isLANHost(uri.Host):
					priority -= 1
				}

				dialTargets = append(dialTargets, dialTarget{
					addr:     addr,
					dialer:   dialer,
					priority: priority,
					deviceID: deviceID,
					uri:      uri,
				})
			}

			conn, ok := s.dialParallel(ctx, deviceCfg.DeviceID, dialTargets)
			if ok {
				s.conns <- conn
			}
		}

		nextDial, sleep = filterAndFindSleepDuration(nextDial, seen, now)

		if initialRampup < sleep {
			l.Debugln("initial rampup; sleep", initialRampup, "and update to", initialRampup*2)
			sleep = initialRampup
			initialRampup *= 2
		} else {
			l.Debugln("sleep until next dial", sleep)
		}

		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *service) isLANHost(host string) bool {
	// Probably we are called with an ip:port combo which we can resolve as
	// a TCP address.
	if addr, err := net.ResolveTCPAddr("tcp", host); err == nil {
		return s.isLAN(addr)
	}
	// ... but this function looks general enough that someone might try
	// with just an IP as well in the future so lets allow that.
	if addr, err := net.ResolveIPAddr("ip", host); err == nil {
		return s.isLAN(addr)
	}
	return false
}

func (s *service) isLAN(addr net.Addr) bool {
	var ip net.IP

	switch addr := addr.(type) {
	case *net.IPAddr:
		ip = addr.IP
	case *net.TCPAddr:
		ip = addr.IP
	case *net.UDPAddr:
		ip = addr.IP
	default:
		// From the standard library, just Unix sockets.
		// If you invent your own, handle it.
		return false
	}

	if ip.IsLoopback() {
		return true
	}

	for _, lan := range s.cfg.Options().AlwaysLocalNets {
		_, ipnet, err := net.ParseCIDR(lan)
		if err != nil {
			l.Debugln("Network", lan, "is malformed:", err)
			continue
		}
		if ipnet.Contains(ip) {
			return true
		}
	}

	lans, _ := osutil.GetLans()
	for _, lan := range lans {
		if lan.Contains(ip) {
			return true
		}
	}

	return false
}

func (s *service) createListener(factory listenerFactory, uri *url.URL) bool {
	// must be called with listenerMut held

	l.Debugln("Starting listener", uri)

	listener := factory.New(uri, s.cfg, s.tlsCfg, s.conns, s.natService)
	listener.OnAddressesChanged(s.logListenAddressesChangedEvent)
	s.listeners[uri.String()] = listener
	s.listenerTokens[uri.String()] = s.listenerSupervisor.Add(listener)
	return true
}

func (s *service) logListenAddressesChangedEvent(l ListenerAddresses) {
	s.evLogger.Log(events.ListenAddressesChanged, map[string]interface{}{
		"address": l.URI,
		"lan":     l.LANAddresses,
		"wan":     l.WANAddresses,
	})
}

func (s *service) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (s *service) CommitConfiguration(from, to config.Configuration) bool {
	newDevices := make(map[protocol.DeviceID]bool, len(to.Devices))
	for _, dev := range to.Devices {
		newDevices[dev.DeviceID] = true
	}

	for _, dev := range from.Devices {
		if !newDevices[dev.DeviceID] {
			warningLimitersMut.Lock()
			delete(warningLimiters, dev.DeviceID)
			warningLimitersMut.Unlock()
		}
	}

	s.listenersMut.Lock()
	seen := make(map[string]struct{})
	for _, addr := range to.Options.ListenAddresses() {
		if addr == "" {
			// We can get an empty address if there is an empty listener
			// element in the config, indicating no listeners should be
			// used. This is not an error.
			continue
		}

		uri, err := url.Parse(addr)
		if err != nil {
			l.Warnf("Skipping malformed listener URL %q: %v", addr, err)
			continue
		}

		// Make sure we always have the canonical representation of the URL.
		// This is for consistency as we use it as a map key, but also to
		// avoid misunderstandings. We do not just use the canonicalized
		// version, because an URL that looks very similar to a human might
		// mean something entirely different to the computer (e.g.,
		// tcp:/127.0.0.1:22000 in fact being equivalent to tcp://:22000).
		if canonical := uri.String(); canonical != addr {
			l.Warnf("Skipping malformed listener URL %q (not canonical)", addr)
			continue
		}

		if _, ok := s.listeners[addr]; ok {
			seen[addr] = struct{}{}
			continue
		}

		factory, err := getListenerFactory(to, uri)
		if errors.Is(err, errUnsupported) {
			l.Debugf("Listener for %v: %v", uri, err)
			continue
		} else if err != nil {
			l.Infof("Listener for %v: %v", uri, err)
			continue
		}

		s.createListener(factory, uri)
		seen[addr] = struct{}{}
	}

	for addr, listener := range s.listeners {
		if _, ok := seen[addr]; !ok || listener.Factory().Valid(to) != nil {
			l.Debugln("Stopping listener", addr)
			s.listenerSupervisor.Remove(s.listenerTokens[addr])
			delete(s.listenerTokens, addr)
			delete(s.listeners, addr)
		}
	}
	s.listenersMut.Unlock()

	return true
}

func (s *service) AllAddresses() []string {
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
	return util.UniqueTrimmedStrings(addrs)
}

func (s *service) ExternalAddresses() []string {
	if s.cfg.Options().AnnounceLANAddresses {
		return s.AllAddresses()
	}
	s.listenersMut.RLock()
	var addrs []string
	for _, listener := range s.listeners {
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	s.listenersMut.RUnlock()
	return util.UniqueTrimmedStrings(addrs)
}

func (s *service) ListenerStatus() map[string]ListenerStatusEntry {
	result := make(map[string]ListenerStatusEntry)
	s.listenersMut.RLock()
	for addr, listener := range s.listeners {
		var status ListenerStatusEntry

		if err := listener.Error(); err != nil {
			errStr := err.Error()
			status.Error = &errStr
		}

		status.LANAddresses = urlsToStrings(listener.LANAddresses())
		status.WANAddresses = urlsToStrings(listener.WANAddresses())

		result[addr] = status
	}
	s.listenersMut.RUnlock()
	return result
}

type connectionStatusHandler struct {
	connectionStatusMut sync.RWMutex
	connectionStatus    map[string]ConnectionStatusEntry // address -> latest error/status
}

func newConnectionStatusHandler() connectionStatusHandler {
	return connectionStatusHandler{
		connectionStatusMut: sync.NewRWMutex(),
		connectionStatus:    make(map[string]ConnectionStatusEntry),
	}
}

func (s *connectionStatusHandler) ConnectionStatus() map[string]ConnectionStatusEntry {
	result := make(map[string]ConnectionStatusEntry)
	s.connectionStatusMut.RLock()
	for k, v := range s.connectionStatus {
		result[k] = v
	}
	s.connectionStatusMut.RUnlock()
	return result
}

func (s *connectionStatusHandler) setConnectionStatus(address string, err error) {
	if errors.Cause(err) == context.Canceled {
		return
	}

	status := ConnectionStatusEntry{When: time.Now().UTC().Truncate(time.Second)}
	if err != nil {
		errStr := err.Error()
		status.Error = &errStr
	}

	s.connectionStatusMut.Lock()
	s.connectionStatus[address] = status
	s.connectionStatusMut.Unlock()
}

func (s *service) NATType() string {
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

func getDialerFactory(cfg config.Configuration, uri *url.URL) (dialerFactory, error) {
	dialerFactory, ok := dialers[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown address scheme %q", uri.Scheme)
	}
	if err := dialerFactory.Valid(cfg); err != nil {
		return nil, err
	}

	return dialerFactory, nil
}

func getListenerFactory(cfg config.Configuration, uri *url.URL) (listenerFactory, error) {
	listenerFactory, ok := listeners[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown address scheme %q", uri.Scheme)
	}
	if err := listenerFactory.Valid(cfg); err != nil {
		return nil, err
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

func (s *service) dialParallel(ctx context.Context, deviceID protocol.DeviceID, dialTargets []dialTarget) (internalConn, bool) {
	// Group targets into buckets by priority
	dialTargetBuckets := make(map[int][]dialTarget, len(dialTargets))
	for _, tgt := range dialTargets {
		dialTargetBuckets[tgt.priority] = append(dialTargetBuckets[tgt.priority], tgt)
	}

	// Get all available priorities
	priorities := make([]int, 0, len(dialTargetBuckets))
	for prio := range dialTargetBuckets {
		priorities = append(priorities, prio)
	}

	// Sort the priorities so that we dial lowest first (which means highest...)
	sort.Ints(priorities)

	for _, prio := range priorities {
		tgts := dialTargetBuckets[prio]
		res := make(chan internalConn, len(tgts))
		wg := stdsync.WaitGroup{}
		for _, tgt := range tgts {
			wg.Add(1)
			go func(tgt dialTarget) {
				conn, err := tgt.Dial(ctx)
				if err == nil {
					// Closes the connection on error
					err = s.validateIdentity(conn, deviceID)
				}
				s.setConnectionStatus(tgt.addr, err)
				if err != nil {
					l.Debugln("dialing", deviceID, tgt.uri, "error:", err)
				} else {
					l.Debugln("dialing", deviceID, tgt.uri, "success:", conn)
					res <- conn
				}
				wg.Done()
			}(tgt)
		}

		// Spawn a routine which will unblock main routine in case we fail
		// to connect to anyone.
		go func() {
			wg.Wait()
			close(res)
		}()

		// Wait for the first connection, or for channel closure.
		if conn, ok := <-res; ok {
			// Got a connection, means more might come back, hence spawn a
			// routine that will do the discarding.
			l.Debugln("connected to", deviceID, prio, "using", conn, conn.priority)
			go func(deviceID protocol.DeviceID, prio int) {
				wg.Wait()
				l.Debugln("discarding", len(res), "connections while connecting to", deviceID, prio)
				for conn := range res {
					conn.Close()
				}
			}(deviceID, prio)
			return conn, ok
		}
		// Failed to connect, report that fact.
		l.Debugln("failed to connect to", deviceID, prio)
	}
	return internalConn{}, false
}

func (s *service) validateIdentity(c internalConn, expectedID protocol.DeviceID) error {
	cs := c.ConnectionState()

	// We should have received exactly one certificate from the other
	// side. If we didn't, they don't have a device ID and we drop the
	// connection.
	certs := cs.PeerCertificates
	if cl := len(certs); cl != 1 {
		l.Infof("Got peer certificate list of length %d != 1 from peer at %s; protocol error", cl, c)
		c.Close()
		return fmt.Errorf("expected 1 certificate, got %d", cl)
	}
	remoteCert := certs[0]
	remoteID := protocol.NewDeviceID(remoteCert.Raw)

	// The device ID should not be that of ourselves. It can happen
	// though, especially in the presence of NAT hairpinning, multiple
	// clients between the same NAT gateway, and global discovery.
	if remoteID == s.myID {
		l.Infof("Connected to myself (%s) at %s - should not happen", remoteID, c)
		c.Close()
		return errors.New("connected to self")
	}

	// We should see the expected device ID
	if !remoteID.Equals(expectedID) {
		c.Close()
		return fmt.Errorf("unexpected device id, expected %s got %s", expectedID, remoteID)
	}

	return nil
}
