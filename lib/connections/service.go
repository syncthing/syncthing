// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/juju/ratelimit"
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
)

var (
	dialers   = make(map[string]dialerFactory, 0)
	listeners = make(map[string]listenerFactory, 0)
)

// Service listens and dials all configured unconnected devices, via supported
// dialers. Successful connections are handed to the model.
type Service struct {
	*suture.Supervisor
	cfg                  *config.Wrapper
	myID                 protocol.DeviceID
	model                Model
	tlsCfg               *tls.Config
	discoverer           discover.Finder
	conns                chan IntermediateConnection
	bepProtocolName      string
	tlsDefaultCommonName string
	lans                 []*net.IPNet
	writeRateLimit       *ratelimit.Bucket
	readRateLimit        *ratelimit.Bucket
	natService           *nat.Service
	natServiceToken      *suture.ServiceToken

	mut               sync.RWMutex
	listeners         map[string]genericListener
	listenerTokens    map[string]suture.ServiceToken
	currentConnection map[protocol.DeviceID]Connection
}

func NewService(cfg *config.Wrapper, myID protocol.DeviceID, mdl Model, tlsCfg *tls.Config, discoverer discover.Finder,
	bepProtocolName string, tlsDefaultCommonName string, lans []*net.IPNet) *Service {

	service := &Service{
		Supervisor:           suture.NewSimple("connections.Service"),
		cfg:                  cfg,
		myID:                 myID,
		model:                mdl,
		tlsCfg:               tlsCfg,
		discoverer:           discoverer,
		conns:                make(chan IntermediateConnection),
		bepProtocolName:      bepProtocolName,
		tlsDefaultCommonName: tlsDefaultCommonName,
		lans:                 lans,
		natService:           nat.NewService(myID, cfg),

		mut:               sync.NewRWMutex(),
		listeners:         make(map[string]genericListener),
		listenerTokens:    make(map[string]suture.ServiceToken),
		currentConnection: make(map[protocol.DeviceID]Connection),
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
	// (handled in configuration changing) to handle incoming connections,
	// one routine to periodically attempt outgoing connections, one routine to
	// the the common handling regardless of whether the connection was
	// incoming or outgoing.

	service.Add(serviceFunc(service.connect))
	service.Add(serviceFunc(service.handle))

	raw := cfg.Raw()
	// Actually starts the listeners and NAT service
	service.CommitConfiguration(raw, raw)

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
			l.Infof("Failed to exchange Hello messages with %s (%s): %s", remoteID, c.RemoteAddr(), err)
			c.Close()
			continue
		}

		s.model.OnHello(remoteID, c.RemoteAddr(), hello)

		// If we have a relay connection, and the new incoming connection is
		// not a relay connection, we should drop that, and prefer the this one.
		s.mut.RLock()
		skip := false
		ct, ok := s.currentConnection[remoteID]

		// Lower priority is better, just like nice etc.
		if ok && ct.Priority > c.Priority {
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
			skip = true
		} else if s.model.IsPaused(remoteID) {
			l.Infof("Connection from paused device (%s)", remoteID)
			c.Close()
			skip = true
		}
		s.mut.RUnlock()
		if skip {
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

				wr := io.Writer(c)
				if limit && s.writeRateLimit != nil {
					wr = NewWriteLimiter(c, s.writeRateLimit)
				}

				rd := io.Reader(c)
				if limit && s.readRateLimit != nil {
					rd = NewReadLimiter(c, s.readRateLimit)
				}

				name := fmt.Sprintf("%s-%s (%s)", c.LocalAddr(), c.RemoteAddr(), c.Type)
				protoConn := protocol.NewConnection(remoteID, rd, wr, s.model, name, deviceCfg.Compression)
				modelConn := Connection{c, protoConn}

				l.Infof("Established secure connection to %s at %s", remoteID, name)
				l.Debugf("cipher suite: %04X in lan: %t", c.ConnectionState().CipherSuite, !limit)

				s.mut.Lock()
				s.model.AddConnection(modelConn, hello)
				s.currentConnection[remoteID] = modelConn
				s.mut.Unlock()
				continue next
			}
		}

		l.Infof("Connection from %s (%s) with ignored device ID %s", c.RemoteAddr(), c.Type, remoteID)
		c.Close()
	}
}

func (s *Service) connect() {
	nextDial := make(map[string]time.Time)
	delay := time.Second
	sleep := time.Second
	for {
		l.Debugln("Reconnect loop")

		now := time.Now()
		var seen []string

	nextDevice:
		for deviceID, deviceCfg := range s.cfg.Devices() {
			if deviceID == s.myID {
				continue
			}

			l.Debugln("Reconnect loop for", deviceID)

			s.mut.RLock()
			paused := s.model.IsPaused(deviceID)
			connected := s.model.ConnectedTo(deviceID)
			ct := s.currentConnection[deviceID]
			s.mut.RUnlock()

			if paused {
				continue
			}

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
				uri, err := url.Parse(addr)
				if err != nil {
					l.Infoln("Failed to parse connection url:", addr, err)
					continue
				}

				dialerFactory, ok := dialers[uri.Scheme]
				if !ok {
					l.Infoln("Unknown address schema", uri)
					continue
				}

				dialer := dialerFactory(s.cfg, s.tlsCfg)

				nextDialAt, ok := nextDial[uri.String()]
				if ok && nextDialAt.After(now) {
					l.Debugln("Not dialing as next dial is at %s and current time is %s", nextDialAt, now)
					continue
				}

				if connected && dialer.Priority() <= ct.Priority {
					l.Debugln("Not dialing using %s as priorty is less than current connection (%d <= %d)", dialer, dialer.Priority(), ct.Priority)
					continue
				}

				nextDial[uri.String()] = now.Add(dialer.RedialFrequency())

				l.Debugln("dial", deviceCfg.DeviceID, uri)
				conn, err := dialer.Dial(deviceID, uri)
				if err != nil {
					l.Debugln("dial failed", deviceCfg.DeviceID, uri, err)
					continue
				}

				if connected {
					s.model.Close(deviceID, fmt.Errorf("switching connections"))
				}

				s.conns <- conn
				continue nextDevice
			}
		}

		nextDial, sleep = filterAndFindSleepDuration(nextDial, seen, now)

		if delay > sleep {
			delay = sleep
		}

		time.Sleep(delay)
		delay *= 2
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

func (s *Service) createListener(addr string) {
	uri, err := url.Parse(addr)
	if err != nil {
		l.Infoln("Failed to parse listen address:", addr, err)
		return
	}

	listenerFactory, ok := listeners[uri.Scheme]
	if !ok {
		l.Infoln("Unknown listen address scheme:", uri.String())
		return
	}

	listener := listenerFactory(uri, s.tlsCfg, s.conns, s.natService)
	listener.OnAddressesChanged(s.logListenAddressesChangedEvent)
	s.mut.Lock()
	s.listeners[addr] = listener
	s.listenerTokens[addr] = s.Add(listener)
	s.mut.Unlock()
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
	// We require a restart if a device as been removed.

	restart := false

	newDevices := make(map[protocol.DeviceID]bool, len(to.Devices))
	for _, dev := range to.Devices {
		newDevices[dev.DeviceID] = true
	}

	for _, dev := range from.Devices {
		if !newDevices[dev.DeviceID] {
			restart = true
		}
	}

	s.mut.RLock()
	existingListeners := s.listeners
	s.mut.RUnlock()

	seen := make(map[string]struct{})

	for _, addr := range config.Wrap("", to).ListenAddresses() {
		if _, ok := existingListeners[addr]; !ok {
			l.Debugln("Staring listener", addr)
			s.createListener(addr)
		}
		seen[addr] = struct{}{}
	}

	s.mut.Lock()
	for addr := range s.listeners {
		if _, ok := seen[addr]; !ok {
			l.Debugln("Stopping listener", addr)
			s.Remove(s.listenerTokens[addr])
			delete(s.listenerTokens, addr)
			delete(s.listeners, addr)
		}
	}
	s.mut.Unlock()

	if to.Options.NATEnabled && s.natServiceToken == nil {
		l.Debugln("Starting NAT service")
		token := s.Add(s.natService)
		s.natServiceToken = &token
	} else if !to.Options.NATEnabled && s.natServiceToken != nil {
		l.Debugln("Stopping NAT service")
		s.Remove(*s.natServiceToken)
		s.natServiceToken = nil
	}

	return !restart
}

func (s *Service) AllAddresses() []string {
	s.mut.RLock()
	var addrs []string
	for _, listener := range s.listeners {
		for _, lanAddr := range listener.LANAddresses() {
			addrs = append(addrs, lanAddr.String())
		}
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	s.mut.RUnlock()
	return util.UniqueStrings(addrs)
}

func (s *Service) ExternalAddresses() []string {
	s.mut.RLock()
	var addrs []string
	for _, listener := range s.listeners {
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	s.mut.RUnlock()
	return util.UniqueStrings(addrs)
}

func (s *Service) Status() map[string]interface{} {
	s.mut.RLock()
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
	s.mut.RUnlock()
	return result
}

func exchangeHello(c net.Conn, h protocol.HelloMessage) (protocol.HelloMessage, error) {
	if err := c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return protocol.HelloMessage{}, err
	}
	defer c.SetDeadline(time.Time{})

	header := make([]byte, 8)
	msg := h.MustMarshalXDR()

	binary.BigEndian.PutUint32(header[:4], protocol.HelloMessageMagic)
	binary.BigEndian.PutUint32(header[4:], uint32(len(msg)))

	if _, err := c.Write(header); err != nil {
		return protocol.HelloMessage{}, err
	}

	if _, err := c.Write(msg); err != nil {
		return protocol.HelloMessage{}, err
	}

	if _, err := io.ReadFull(c, header); err != nil {
		return protocol.HelloMessage{}, err
	}

	if binary.BigEndian.Uint32(header[:4]) != protocol.HelloMessageMagic {
		return protocol.HelloMessage{}, fmt.Errorf("incorrect magic")
	}

	msgSize := binary.BigEndian.Uint32(header[4:])
	if msgSize > 1024 {
		return protocol.HelloMessage{}, fmt.Errorf("hello message too big")
	}

	buf := make([]byte, msgSize)

	var hello protocol.HelloMessage

	if _, err := io.ReadFull(c, buf); err != nil {
		return protocol.HelloMessage{}, err
	}

	if err := hello.UnmarshalXDR(buf); err != nil {
		return protocol.HelloMessage{}, err
	}

	return hello, nil
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
