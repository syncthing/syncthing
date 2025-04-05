// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate -command counterfeiter go run github.com/maxbrunsfeld/counterfeiter/v6
//go:generate counterfeiter -o mocks/service.go --fake-name Service . Service

package connections

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"slices"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"github.com/thejerf/suture/v4"
	"golang.org/x/time/rate"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/sliceutil"
	"github.com/syncthing/syncthing/lib/stringutil"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"

	// Registers NAT service providers
	_ "github.com/syncthing/syncthing/lib/pmp"
	_ "github.com/syncthing/syncthing/lib/upnp"
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

	// Various reasons to reject a connection
	errNetworkNotAllowed      = errors.New("network not allowed")
	errDeviceAlreadyConnected = errors.New("already connected to this device")
	errDeviceIgnored          = errors.New("device is ignored")
	errConnLimitReached       = errors.New("connection limit reached")
	errDevicePaused           = errors.New("device is paused")

	// A connection is being closed to make space for better ones
	errReplacingConnection = errors.New("replacing connection")
)

const (
	perDeviceWarningIntv          = 15 * time.Minute
	tlsHandshakeTimeout           = 10 * time.Second
	minConnectionLoopSleep        = 5 * time.Second
	stdConnectionLoopSleep        = time.Minute
	worstDialerPriority           = math.MaxInt32
	recentlySeenCutoff            = 7 * 24 * time.Hour
	shortLivedConnectionThreshold = 5 * time.Second
	dialMaxParallel               = 64
	dialMaxParallelPerDevice      = 8
	maxNumConnections             = 128 // the maximum number of connections we maintain to any given device
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

type connWithHello struct {
	c          internalConn
	hello      protocol.Hello
	err        error
	remoteID   protocol.DeviceID
	remoteCert *x509.Certificate
}

type service struct {
	*suture.Supervisor
	connectionStatusHandler
	deviceConnectionTracker

	cfg                  config.Wrapper
	myID                 protocol.DeviceID
	model                Model
	tlsCfg               *tls.Config
	discoverer           discover.Finder
	conns                chan internalConn
	hellos               chan *connWithHello
	bepProtocolName      string
	tlsDefaultCommonName string
	limiter              *limiter
	natService           *nat.Service
	evLogger             events.Logger
	registry             *registry.Registry
	keyGen               *protocol.KeyGenerator
	lanChecker           *lanChecker

	dialNow           chan struct{}
	dialNowDevices    map[protocol.DeviceID]struct{}
	dialNowDevicesMut sync.Mutex

	listenersMut   sync.RWMutex
	listeners      map[string]genericListener
	listenerTokens map[string]suture.ServiceToken
}

func NewService(cfg config.Wrapper, myID protocol.DeviceID, mdl Model, tlsCfg *tls.Config, discoverer discover.Finder, bepProtocolName string, tlsDefaultCommonName string, evLogger events.Logger, registry *registry.Registry, keyGen *protocol.KeyGenerator) Service {
	spec := svcutil.SpecWithInfoLogger(l)
	service := &service{
		Supervisor:              suture.New("connections.Service", spec),
		connectionStatusHandler: newConnectionStatusHandler(),

		cfg:                  cfg,
		myID:                 myID,
		model:                mdl,
		tlsCfg:               tlsCfg,
		discoverer:           discoverer,
		conns:                make(chan internalConn),
		hellos:               make(chan *connWithHello),
		bepProtocolName:      bepProtocolName,
		tlsDefaultCommonName: tlsDefaultCommonName,
		limiter:              newLimiter(myID, cfg),
		natService:           nat.NewService(myID, cfg),
		evLogger:             evLogger,
		registry:             registry,
		keyGen:               keyGen,
		lanChecker:           &lanChecker{cfg},

		dialNowDevicesMut: sync.NewMutex(),
		dialNow:           make(chan struct{}, 1),
		dialNowDevices:    make(map[protocol.DeviceID]struct{}),

		listenersMut:   sync.NewRWMutex(),
		listeners:      make(map[string]genericListener),
		listenerTokens: make(map[string]suture.ServiceToken),
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

	service.Add(svcutil.AsService(service.connect, fmt.Sprintf("%s/connect", service)))
	service.Add(svcutil.AsService(service.handleConns, fmt.Sprintf("%s/handleConns", service)))
	service.Add(svcutil.AsService(service.handleHellos, fmt.Sprintf("%s/handleHellos", service)))
	service.Add(service.natService)

	svcutil.OnSupervisorDone(service.Supervisor, func() {
		service.cfg.Unsubscribe(service.limiter)
		service.cfg.Unsubscribe(service)
	})

	return service
}

func (s *service) handleConns(ctx context.Context) error {
	for {
		var c internalConn
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
		if cs.NegotiatedProtocol != s.bepProtocolName {
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
			l.Debugf("Connected to myself (%s) at %s", remoteID, c)
			c.Close()
			continue
		}

		if err := s.connectionCheckEarly(remoteID, c); err != nil {
			if errors.Is(err, errDeviceAlreadyConnected) {
				l.Debugf("Connection from %s at %s (%s) rejected: %v", remoteID, c.RemoteAddr(), c.Type(), err)
			} else {
				l.Infof("Connection from %s at %s (%s) rejected: %v", remoteID, c.RemoteAddr(), c.Type(), err)
			}
			c.Close()
			continue
		}

		_ = c.SetDeadline(time.Now().Add(20 * time.Second))
		go func() {
			// Exchange Hello messages with the peer.
			outgoing := s.helloForDevice(remoteID)
			incoming, err := protocol.ExchangeHello(c, outgoing)
			// The timestamps are used to create the connection ID.
			c.connectionID = newConnectionID(outgoing.Timestamp, incoming.Timestamp)

			select {
			case s.hellos <- &connWithHello{c, incoming, err, remoteID, remoteCert}:
			case <-ctx.Done():
			}
		}()
	}
}

func (s *service) helloForDevice(remoteID protocol.DeviceID) protocol.Hello {
	hello := protocol.Hello{
		ClientName:    "syncthing",
		ClientVersion: build.Version,
		Timestamp:     time.Now().UnixNano(),
	}
	if cfg, ok := s.cfg.Device(remoteID); ok {
		hello.NumConnections = cfg.NumConnections()
		// Set our name (from the config of our device ID) only if we
		// already know about the other side device ID.
		if myCfg, ok := s.cfg.Device(s.myID); ok {
			hello.DeviceName = myCfg.Name
		}
	}
	return hello
}

func (s *service) connectionCheckEarly(remoteID protocol.DeviceID, c internalConn) error {
	if s.cfg.IgnoredDevice(remoteID) {
		return errDeviceIgnored
	}

	if max := s.cfg.Options().ConnectionLimitMax; max > 0 && s.numConnectedDevices() >= max {
		// We're not allowed to accept any more connections.
		return errConnLimitReached
	}

	cfg, ok := s.cfg.Device(remoteID)
	if !ok {
		// We do go ahead exchanging hello messages to get information about the device.
		return nil
	}

	if cfg.Paused {
		return errDevicePaused
	}

	if len(cfg.AllowedNetworks) > 0 && !IsAllowedNetwork(c.RemoteAddr().String(), cfg.AllowedNetworks) {
		// The connection is not from an allowed network.
		return errNetworkNotAllowed
	}

	currentConns := s.numConnectionsForDevice(cfg.DeviceID)
	desiredConns := s.desiredConnectionsToDevice(cfg.DeviceID)
	worstPrio := s.worstConnectionPriority(remoteID)
	ourUpgradeThreshold := c.priority + s.cfg.Options().ConnectionPriorityUpgradeThreshold
	if currentConns >= desiredConns && ourUpgradeThreshold >= worstPrio {
		l.Debugf("Not accepting connection to %s at %s: already have %d connections, desire %d", remoteID, c, currentConns, desiredConns)
		return errDeviceAlreadyConnected
	}

	return nil
}

func (s *service) handleHellos(ctx context.Context) error {
	for {
		var c internalConn
		var hello protocol.Hello
		var err error
		var remoteID protocol.DeviceID
		var remoteCert *x509.Certificate

		select {
		case <-ctx.Done():
			return ctx.Err()
		case withHello := <-s.hellos:
			c = withHello.c
			hello = withHello.hello
			err = withHello.err
			remoteID = withHello.remoteID
			remoteCert = withHello.remoteCert
		}

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
		rd, wr := s.limiter.getLimiters(remoteID, c, c.IsLocal())

		protoConn := protocol.NewConnection(remoteID, rd, wr, c, s.model, c, deviceCfg.Compression.ToProtocol(), s.cfg.FolderPasswords(remoteID), s.keyGen)
		s.accountAddedConnection(protoConn, hello, s.cfg.Options().ConnectionPriorityUpgradeThreshold)
		go func() {
			<-protoConn.Closed()
			s.accountRemovedConnection(protoConn)
			s.dialNowDevicesMut.Lock()
			s.dialNowDevices[remoteID] = struct{}{}
			s.scheduleDialNow()
			s.dialNowDevicesMut.Unlock()
		}()

		l.Infof("Established secure connection to %s at %s", remoteID.Short(), c)

		s.model.AddConnection(protoConn, hello)
		continue
	}
}

func (s *service) connect(ctx context.Context) error {
	// Map of when to earliest dial each given device + address again
	nextDialAt := make(nextDialRegistry)

	// Used as delay for the first few connection attempts (adjusted up to
	// minConnectionLoopSleep), increased exponentially until it reaches
	// stdConnectionLoopSleep, at which time the normal sleep mechanism
	// kicks in.
	initialRampup := time.Second

	for {
		cfg := s.cfg.RawCopy()
		bestDialerPriority := s.bestDialerPriority(cfg)
		isInitialRampup := initialRampup < stdConnectionLoopSleep

		l.Debugln("Connection loop")
		if isInitialRampup {
			l.Debugln("Connection loop in initial rampup")
		}

		// Used for consistency throughout this loop run, as time passes
		// while we try connections etc.
		now := time.Now()

		// Attempt to dial all devices that are unconnected or can be connection-upgraded
		s.dialDevices(ctx, now, cfg, bestDialerPriority, nextDialAt, isInitialRampup)

		var sleep time.Duration
		if isInitialRampup {
			// We are in the initial rampup time, so we slowly, statically
			// increase the sleep time.
			sleep = initialRampup
			initialRampup *= 2
		} else {
			// The sleep time is until the next dial scheduled in nextDialAt,
			// clamped by stdConnectionLoopSleep as we don't want to sleep too
			// long (config changes might happen).
			sleep = nextDialAt.sleepDurationAndCleanup(now)
		}

		// ... while making sure not to loop too quickly either.
		if sleep < minConnectionLoopSleep {
			sleep = minConnectionLoopSleep
		}

		l.Debugln("Next connection loop in", sleep)

		timeout := time.NewTimer(sleep)
		select {
		case <-s.dialNow:
			// Remove affected devices from nextDialAt to dial immediately,
			// regardless of when we last dialed it (there's cool down in the
			// registry for too many repeat dials).
			s.dialNowDevicesMut.Lock()
			for device := range s.dialNowDevices {
				nextDialAt.redialDevice(device, now)
			}
			s.dialNowDevices = make(map[protocol.DeviceID]struct{})
			s.dialNowDevicesMut.Unlock()
			timeout.Stop()
		case <-timeout.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *service) bestDialerPriority(cfg config.Configuration) int {
	bestDialerPriority := worstDialerPriority
	for _, df := range dialers {
		if df.Valid(cfg) != nil {
			continue
		}
		prio := df.New(cfg.Options, s.tlsCfg, s.registry, s.lanChecker).Priority("127.0.0.1")
		if prio < bestDialerPriority {
			bestDialerPriority = prio
		}
	}
	return bestDialerPriority
}

func (s *service) dialDevices(ctx context.Context, now time.Time, cfg config.Configuration, bestDialerPriority int, nextDialAt nextDialRegistry, initial bool) {
	// Figure out current connection limits up front to see if there's any
	// point in resolving devices and such at all.
	allowAdditional := 0 // no limit
	connectionLimit := cfg.Options.LowestConnectionLimit()
	if connectionLimit > 0 {
		current := s.numConnectedDevices()
		allowAdditional = connectionLimit - current
		if allowAdditional <= 0 {
			l.Debugf("Skipping dial because we've reached the connection limit, current %d >= limit %d", current, connectionLimit)
			return
		}
	}

	// Get device statistics for the last seen time of each device. This
	// isn't critical, so ignore the potential error.
	stats, _ := s.model.DeviceStatistics()

	queue := make(dialQueue, 0, len(cfg.Devices))
	for _, deviceCfg := range cfg.Devices {
		// Don't attempt to connect to ourselves...
		if deviceCfg.DeviceID == s.myID {
			continue
		}

		// Don't attempt to connect to paused devices...
		if deviceCfg.Paused {
			continue
		}

		// See if we are already connected and, if so, what our cutoff is
		// for dialer priority.
		priorityCutoff := worstDialerPriority
		if currentConns := s.numConnectionsForDevice(deviceCfg.DeviceID); currentConns > 0 {
			// Set the priority cutoff to the current connection's priority,
			// so that we don't attempt any dialers with worse priority.
			priorityCutoff = s.worstConnectionPriority(deviceCfg.DeviceID)

			// Reduce the priority cutoff by the upgrade threshold, so that
			// we don't attempt dialers that aren't considered a worthy upgrade.
			priorityCutoff -= cfg.Options.ConnectionPriorityUpgradeThreshold

			if bestDialerPriority >= priorityCutoff && currentConns >= s.desiredConnectionsToDevice(deviceCfg.DeviceID) {
				// Our best dialer is not any better than what we already
				// have, and we already have the desired number of
				// connections to this device,so nothing to do here.
				l.Debugf("Skipping dial to %s because we already have %d connections and our best dialer is not better than %d", deviceCfg.DeviceID.Short(), currentConns, priorityCutoff)
				continue
			}
		}

		dialTargets := s.resolveDialTargets(ctx, now, cfg, deviceCfg, nextDialAt, initial, priorityCutoff)
		if len(dialTargets) > 0 {
			queue = append(queue, dialQueueEntry{
				id:         deviceCfg.DeviceID,
				lastSeen:   stats[deviceCfg.DeviceID].LastSeen,
				shortLived: stats[deviceCfg.DeviceID].LastConnectionDurationS < shortLivedConnectionThreshold.Seconds(),
				targets:    dialTargets,
			})
		}
	}

	// Sort the queue in an order we think will be useful (most recent
	// first, deprioritising unstable devices, randomizing those we haven't
	// seen in a long while). If we don't do connection limiting the sorting
	// doesn't have much effect, but it may result in getting up and running
	// quicker if only a subset of configured devices are actually reachable
	// (by prioritizing those that were reachable recently).
	queue.Sort()

	// Perform dials according to the queue, stopping when we've reached the
	// allowed additional number of connections (if limited).
	numConns := 0
	var numConnsMut stdsync.Mutex
	dialSemaphore := semaphore.New(dialMaxParallel)
	dialWG := new(stdsync.WaitGroup)
	dialCtx, dialCancel := context.WithCancel(ctx)
	defer func() {
		dialWG.Wait()
		dialCancel()
	}()
	for i := range queue {
		select {
		case <-dialCtx.Done():
			return
		default:
		}
		dialWG.Add(1)
		go func(entry dialQueueEntry) {
			defer dialWG.Done()
			conn, ok := s.dialParallel(dialCtx, entry.id, entry.targets, dialSemaphore)
			if !ok {
				return
			}
			numConnsMut.Lock()
			if allowAdditional == 0 || numConns < allowAdditional {
				select {
				case s.conns <- conn:
					numConns++
					if allowAdditional > 0 && numConns >= allowAdditional {
						dialCancel()
					}
				case <-dialCtx.Done():
				}
			}
			numConnsMut.Unlock()
		}(queue[i])
	}
}

func (s *service) resolveDialTargets(ctx context.Context, now time.Time, cfg config.Configuration, deviceCfg config.DeviceConfiguration, nextDialAt nextDialRegistry, initial bool, priorityCutoff int) []dialTarget {
	deviceID := deviceCfg.DeviceID

	addrs := s.resolveDeviceAddrs(ctx, deviceCfg)
	l.Debugln("Resolved device", deviceID.Short(), "addresses:", addrs)

	dialTargets := make([]dialTarget, 0, len(addrs))
	for _, addr := range addrs {
		// Use both device and address, as you might have two devices connected
		// to the same relay
		if !initial && nextDialAt.get(deviceID, addr).After(now) {
			l.Debugf("Not dialing %s via %v as it's not time yet", deviceID.Short(), addr)
			continue
		}

		// If we fail at any step before actually getting the dialer
		// retry in a minute
		nextDialAt.set(deviceID, addr, now.Add(time.Minute))

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

		dialer := dialerFactory.New(s.cfg.Options(), s.tlsCfg, s.registry, s.lanChecker)
		priority := dialer.Priority(uri.Host)
		currentConns := s.numConnectionsForDevice(deviceCfg.DeviceID)
		if priority > priorityCutoff {
			l.Debugf("Not dialing %s at %s using %s as priority is worse than current connection (%d > %d)", deviceID.Short(), addr, dialerFactory, priority, priorityCutoff)
			continue
		}
		if currentConns > 0 && !dialer.AllowsMultiConns() {
			l.Debugf("Not dialing %s at %s using %s as it does not allow multiple connections and we already have a connection", deviceID.Short(), addr, dialerFactory)
			continue
		}
		if currentConns >= s.desiredConnectionsToDevice(deviceCfg.DeviceID) && priority == priorityCutoff {
			l.Debugf("Not dialing %s at %s using %s as priority is equal and we already have %d/%d connections", deviceID.Short(), addr, dialerFactory, currentConns, deviceCfg.NumConnections)
			continue
		}

		nextDialAt.set(deviceID, addr, now.Add(dialer.RedialFrequency()))

		dialTargets = append(dialTargets, dialTarget{
			addr:     addr,
			dialer:   dialer,
			priority: priority,
			deviceID: deviceID,
			uri:      uri,
		})
	}

	return dialTargets
}

func (s *service) resolveDeviceAddrs(ctx context.Context, cfg config.DeviceConfiguration) []string {
	var addrs []string
	for _, addr := range cfg.Addresses {
		if addr == "dynamic" {
			if s.discoverer != nil {
				if t, err := s.discoverer.Lookup(ctx, cfg.DeviceID); err == nil {
					addrs = append(addrs, t...)
				}
			}
		} else {
			addrs = append(addrs, addr)
		}
	}
	return stringutil.UniqueTrimmedStrings(addrs)
}

type lanChecker struct {
	cfg config.Wrapper
}

func (s *lanChecker) isLANHost(host string) bool {
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

func (s *lanChecker) isLAN(addr net.Addr) bool {
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

	if ip.IsLinkLocalUnicast() {
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

	lans, err := osutil.GetInterfaceAddrs(false)
	if err != nil {
		l.Debugln("Failed to retrieve interface IPs:", err)
		priv := ip.IsPrivate()
		l.Debugf("Assuming isLAN=%v for IP %v", priv, ip)
		return priv
	}

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

	listener := factory.New(uri, s.cfg, s.tlsCfg, s.conns, s.natService, s.registry, s.lanChecker)
	listener.OnAddressesChanged(s.logListenAddressesChangedEvent)

	// Retrying a listener many times in rapid succession is unlikely to help,
	// thus back off quickly. A listener may soon be functional again, e.g. due
	// to a network interface coming back online - retry every minute.
	spec := svcutil.SpecWithInfoLogger(l)
	spec.FailureThreshold = 2
	spec.FailureBackoff = time.Minute
	sup := suture.New(fmt.Sprintf("listenerSupervisor@%v", listener), spec)
	sup.Add(listener)

	s.listeners[uri.String()] = listener
	s.listenerTokens[uri.String()] = s.Add(sup)
	return true
}

func (s *service) logListenAddressesChangedEvent(l ListenerAddresses) {
	s.evLogger.Log(events.ListenAddressesChanged, map[string]interface{}{
		"address": l.URI,
		"lan":     l.LANAddresses,
		"wan":     l.WANAddresses,
	})
}

func (s *service) CommitConfiguration(from, to config.Configuration) bool {
	newDevices := make(map[protocol.DeviceID]bool, len(to.Devices))
	for _, dev := range to.Devices {
		newDevices[dev.DeviceID] = true
		registerDeviceMetrics(dev.DeviceID.String())
	}

	for _, dev := range from.Devices {
		if !newDevices[dev.DeviceID] {
			warningLimitersMut.Lock()
			delete(warningLimiters, dev.DeviceID)
			warningLimitersMut.Unlock()
			metricDeviceActiveConnections.DeleteLabelValues(dev.DeviceID.String())
		}
	}

	s.checkAndSignalConnectLoopOnUpdatedDevices(from, to)

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
			s.Remove(s.listenerTokens[addr])
			delete(s.listenerTokens, addr)
			delete(s.listeners, addr)
		}
	}
	s.listenersMut.Unlock()

	return true
}

func (s *service) checkAndSignalConnectLoopOnUpdatedDevices(from, to config.Configuration) {
	oldDevices := from.DeviceMap()
	dial := false
	s.dialNowDevicesMut.Lock()
	for _, dev := range to.Devices {
		if dev.Paused {
			continue
		}
		if oldDev, ok := oldDevices[dev.DeviceID]; !ok || oldDev.Paused {
			s.dialNowDevices[dev.DeviceID] = struct{}{}
			dial = true
		} else if !slices.Equal(oldDev.Addresses, dev.Addresses) {
			dial = true
		}
	}
	if dial {
		s.scheduleDialNow()
	}
	s.dialNowDevicesMut.Unlock()
}

func (s *service) scheduleDialNow() {
	select {
	case s.dialNow <- struct{}{}:
	default:
		// channel is blocked - a config update is already pending for the connection loop.
	}
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
	return stringutil.UniqueTrimmedStrings(addrs)
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
	return stringutil.UniqueTrimmedStrings(addrs)
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
	if errors.Is(err, context.Canceled) {
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

func urlsToStrings(urls []*url.URL) []string {
	strings := make([]string, len(urls))
	for i, url := range urls {
		strings[i] = url.String()
	}
	return strings
}

var (
	warningLimiters    = make(map[protocol.DeviceID]*rate.Limiter)
	warningLimitersMut = sync.NewMutex()
)

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

func (s *service) dialParallel(ctx context.Context, deviceID protocol.DeviceID, dialTargets []dialTarget, parentSema *semaphore.Semaphore) (internalConn, bool) {
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

	sema := semaphore.MultiSemaphore{semaphore.New(dialMaxParallelPerDevice), parentSema}
	for _, prio := range priorities {
		tgts := dialTargetBuckets[prio]
		res := make(chan internalConn, len(tgts))
		wg := stdsync.WaitGroup{}
		for _, tgt := range tgts {
			sema.Take(1)
			wg.Add(1)
			go func(tgt dialTarget) {
				defer func() {
					wg.Done()
					sema.Give(1)
				}()
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
		l.Debugf("Connected to myself (%s) at %s", remoteID, c)
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

type nextDialRegistry map[protocol.DeviceID]nextDialDevice

type nextDialDevice struct {
	nextDial              map[string]time.Time
	coolDownIntervalStart time.Time
	attempts              int
}

func (r nextDialRegistry) get(device protocol.DeviceID, addr string) time.Time {
	return r[device].nextDial[addr]
}

const (
	dialCoolDownInterval    = 2 * time.Minute
	dialCoolDownDelay       = 5 * time.Minute
	dialCoolDownMaxAttempts = 3
)

// redialDevice marks the device for immediate redial, unless the remote keeps
// dropping established connections. Thus we keep track of when the first forced
// re-dial happened, and how many attempts happen in the dialCoolDownInterval
// after that. If it's more than dialCoolDownMaxAttempts, don't force-redial
// that device for dialCoolDownDelay (regular dials still happen).
func (r nextDialRegistry) redialDevice(device protocol.DeviceID, now time.Time) {
	dev, ok := r[device]
	if !ok {
		r[device] = nextDialDevice{
			nextDial:              make(map[string]time.Time),
			coolDownIntervalStart: now,
			attempts:              1,
		}
		return
	}
	if dev.attempts == 0 || now.Before(dev.coolDownIntervalStart.Add(dialCoolDownInterval)) {
		if dev.attempts >= dialCoolDownMaxAttempts {
			// Device has been force redialed too often - let it cool down.
			return
		}
		if dev.attempts == 0 {
			dev.coolDownIntervalStart = now
		}
		dev.attempts++
		dev.nextDial = make(map[string]time.Time)
		r[device] = dev
		return
	}
	if dev.attempts >= dialCoolDownMaxAttempts && now.Before(dev.coolDownIntervalStart.Add(dialCoolDownDelay)) {
		return // Still cooling down
	}
	delete(r, device)
}

func (r nextDialRegistry) set(device protocol.DeviceID, addr string, next time.Time) {
	if _, ok := r[device]; !ok {
		r[device] = nextDialDevice{nextDial: make(map[string]time.Time)}
	}
	r[device].nextDial[addr] = next
}

func (r nextDialRegistry) sleepDurationAndCleanup(now time.Time) time.Duration {
	sleep := stdConnectionLoopSleep
	for id, dev := range r {
		for address, next := range dev.nextDial {
			if next.Before(now) {
				// Expired entry, address was not seen in last pass(es)
				delete(dev.nextDial, address)
				continue
			}
			if cur := next.Sub(now); cur < sleep {
				sleep = cur
			}
		}
		if dev.attempts > 0 {
			interval := dialCoolDownInterval
			if dev.attempts >= dialCoolDownMaxAttempts {
				interval = dialCoolDownDelay
			}
			if now.After(dev.coolDownIntervalStart.Add(interval)) {
				dev.attempts = 0
			}
		}
		if len(dev.nextDial) == 0 && dev.attempts == 0 {
			delete(r, id)
		}
	}
	return sleep
}

func (s *service) desiredConnectionsToDevice(deviceID protocol.DeviceID) int {
	cfg, ok := s.cfg.Device(deviceID)
	if !ok {
		// We want no connections to an unknown device.
		return 0
	}

	otherSide := s.wantConnectionsForDevice(deviceID)
	thisSide := cfg.NumConnections()
	switch {
	case otherSide <= 0:
		// The other side doesn't support multiple connections, or we
		// haven't yet connected to them so we don't know what they support
		// or not. Use a single connection until we know better.
		return 1

	case otherSide == 1:
		// The other side supports multiple connections, but only wants
		// one. We should honour that.
		return 1

	case thisSide == 1:
		// We want only one connection, so we should honour that.
		return 1

	// Finally, we allow negotiation and use the higher of the two values,
	// while keeping at or below the max allowed value.
	default:
		return min(max(thisSide, otherSide), maxNumConnections)
	}
}

// The deviceConnectionTracker keeps track of how many devices we are
// connected to and how many connections we have to each device. It also
// tracks how many connections they are willing to use.
type deviceConnectionTracker struct {
	connectionsMut  stdsync.Mutex
	connections     map[protocol.DeviceID][]protocol.Connection // current connections
	wantConnections map[protocol.DeviceID]int                   // number of connections they want
}

func (c *deviceConnectionTracker) accountAddedConnection(conn protocol.Connection, h protocol.Hello, upgradeThreshold int) {
	c.connectionsMut.Lock()
	defer c.connectionsMut.Unlock()
	// Lazily initialize the maps
	if c.connections == nil {
		c.connections = make(map[protocol.DeviceID][]protocol.Connection)
		c.wantConnections = make(map[protocol.DeviceID]int)
	}
	// Add the connection to the list of current connections and remember
	// how many total connections they want
	d := conn.DeviceID()
	c.connections[d] = append(c.connections[d], conn)
	c.wantConnections[d] = int(h.NumConnections)
	l.Debugf("Added connection for %s (now %d), they want %d connections", d.Short(), len(c.connections[d]), h.NumConnections)

	// Update active connections metric
	metricDeviceActiveConnections.WithLabelValues(d.String()).Inc()

	// Close any connections we no longer want to retain.
	c.closeWorsePriorityConnectionsLocked(d, conn.Priority()-upgradeThreshold)
}

func (c *deviceConnectionTracker) accountRemovedConnection(conn protocol.Connection) {
	c.connectionsMut.Lock()
	defer c.connectionsMut.Unlock()
	d := conn.DeviceID()
	cid := conn.ConnectionID()
	// Remove the connection from the list of current connections
	for i, conn := range c.connections[d] {
		if conn.ConnectionID() == cid {
			c.connections[d] = sliceutil.RemoveAndZero(c.connections[d], i)
			break
		}
	}
	// Clean up if required
	if len(c.connections[d]) == 0 {
		delete(c.connections, d)
		delete(c.wantConnections, d)
	}

	// Update active connections metric
	metricDeviceActiveConnections.WithLabelValues(d.String()).Dec()

	l.Debugf("Removed connection for %s (now %d)", d.Short(), c.connections[d])
}

func (c *deviceConnectionTracker) numConnectionsForDevice(d protocol.DeviceID) int {
	c.connectionsMut.Lock()
	defer c.connectionsMut.Unlock()
	return len(c.connections[d])
}

func (c *deviceConnectionTracker) wantConnectionsForDevice(d protocol.DeviceID) int {
	c.connectionsMut.Lock()
	defer c.connectionsMut.Unlock()
	return c.wantConnections[d]
}

func (c *deviceConnectionTracker) numConnectedDevices() int {
	c.connectionsMut.Lock()
	defer c.connectionsMut.Unlock()
	return len(c.connections)
}

func (c *deviceConnectionTracker) worstConnectionPriority(d protocol.DeviceID) int {
	c.connectionsMut.Lock()
	defer c.connectionsMut.Unlock()
	if len(c.connections[d]) == 0 {
		return math.MaxInt // worst possible priority
	}
	worstPriority := c.connections[d][0].Priority()
	for _, conn := range c.connections[d][1:] {
		if p := conn.Priority(); p > worstPriority {
			worstPriority = p
		}
	}
	return worstPriority
}

// closeWorsePriorityConnectionsLocked closes all connections to the given
// device that are worse than the cutoff priority. Must be called with the
// lock held.
func (c *deviceConnectionTracker) closeWorsePriorityConnectionsLocked(d protocol.DeviceID, cutoff int) {
	for _, conn := range c.connections[d] {
		if p := conn.Priority(); p > cutoff {
			l.Debugf("Closing connection %s to %s with priority %d (cutoff %d)", conn, d.Short(), p, cutoff)
			go conn.Close(errReplacingConnection)
		}
	}
}

// newConnectionID generates a connection ID. The connection ID is designed
// to be unique for each connection and chronologically sortable. It is
// based on the sum of two timestamps: when we think the connection was
// started, and when the other side thinks the connection was started. We
// then add some random data for good measure. This way, even if the other
// side does some funny business with the timestamp, we will get no worse
// than random connection IDs.
func newConnectionID(t0, t1 int64) string {
	var buf [16]byte // 8 bytes timestamp, 8 bytes random
	binary.BigEndian.PutUint64(buf[:], uint64(t0+t1))
	_, _ = io.ReadFull(rand.Reader, buf[8:])
	enc := base32.HexEncoding.WithPadding(base32.NoPadding)
	// We encode the two parts separately and concatenate the results. The
	// reason for this is that the timestamp (64 bits) doesn't precisely
	// align to the base32 encoding (5 bits per character), so we'd get a
	// character in the middle that is a mix of bits from the timestamp and
	// from the random. We want the timestamp part deterministic.
	return enc.EncodeToString(buf[:8]) + enc.EncodeToString(buf[8:])
}
