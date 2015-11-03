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
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"

	"github.com/thejerf/suture"
)

type IntermediateConnection struct {
	*tls.Conn
	Type     string
	Priority int
}

type Connection struct {
	IntermediateConnection
	protocol.Connection
}

type genericDialer interface {
	Dial(protocol.DeviceID, *url.URL, *tls.Config) (IntermediateConnection, error)
	Priority() int
}

type listenerFactory func(*url.URL, *tls.Config, chan IntermediateConnection, *nat.Service) genericListener

type genericListener interface {
	Serve()
	Stop()
	// A given address can potentially be mutated by the listener.
	// For example we bind to tcp://0.0.0.0, but that for example might return
	// tcp://gateway1.ip and tcp://gateway2.ip as WAN addresses due to there
	// being multiple gateways, and us managing to get a UPnP mapping on both
	// and tcp://192.168.0.1 and tcp://10.0.0.1 due to there being multiple
	// network interfaces. (The later case for LAN addresses is made up just
	// to provide an example)
	WANAddresses() []*url.URL
	LANAddresses() []*url.URL
	Error() error
	Details() interface{}
}

var (
	dialers   = make(map[string]genericDialer, 0)
	listeners = make(map[string]listenerFactory, 0)
)

type Model interface {
	protocol.Model
	AddConnection(conn Connection)
	ConnectedTo(remoteID protocol.DeviceID) bool
	IsPaused(remoteID protocol.DeviceID) bool
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
	conns                chan IntermediateConnection
	bepProtocolName      string
	tlsDefaultCommonName string
	lans                 []*net.IPNet
	writeRateLimit       *ratelimit.Bucket
	readRateLimit        *ratelimit.Bucket
	listeners            map[string]genericListener
	natService           *nat.Service

	mut               sync.RWMutex
	currentConnection map[protocol.DeviceID]Connection
}

func NewService(cfg *config.Wrapper, myID protocol.DeviceID, mdl Model, tlsCfg *tls.Config, discoverer discover.Finder, natService *nat.Service,
	bepProtocolName string, tlsDefaultCommonName string, lans []*net.IPNet) *Service {
	service := &Service{
		Supervisor:           suture.NewSimple("connectionService"),
		cfg:                  cfg,
		myID:                 myID,
		model:                mdl,
		tlsCfg:               tlsCfg,
		discoverer:           discoverer,
		conns:                make(chan IntermediateConnection),
		bepProtocolName:      bepProtocolName,
		tlsDefaultCommonName: tlsDefaultCommonName,
		lans:                 lans,
		listeners:            make(map[string]genericListener),
		natService:           natService,

		currentConnection: make(map[protocol.DeviceID]Connection),
	}
	cfg.Subscribe(service)

	if service.cfg.Options().MaxSendKbps > 0 {
		service.writeRateLimit = ratelimit.NewBucketWithRate(float64(1000*service.cfg.Options().MaxSendKbps), int64(5*1000*service.cfg.Options().MaxSendKbps))
	}
	if service.cfg.Options().MaxRecvKbps > 0 {
		service.readRateLimit = ratelimit.NewBucketWithRate(float64(1000*service.cfg.Options().MaxRecvKbps), int64(5*1000*service.cfg.Options().MaxRecvKbps))
	}

	// There are several moving parts here; one routine per listening address
	// to handle incoming connections, one routine to periodically attempt
	// outgoing connections, one routine to the the common handling
	// regardless of whether the connection was incoming or outgoing.
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

		listenerFactory, ok := listeners[uri.Scheme]
		if !ok {
			l.Infoln("Unknown listen address scheme:", uri.String())
			continue
		}

		l.Debugln("listening on", uri)

		listener := listenerFactory(uri, service.tlsCfg, service.conns, natService)
		service.listeners[addr] = listener
		service.Add(listener)
	}
	service.Add(serviceFunc(service.handle))

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

		// If we have a relay connection, and the new incoming connection is
		// not a relay connection, we should drop that, and prefer the this one.
		s.mut.RLock()
		ct, ok := s.currentConnection[remoteID]
		s.mut.RUnlock()

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

				l.Infof("Established secure connection to %s at %s", remoteID, name)
				l.Debugf("cipher suite: %04X in lan: %t", c.ConnectionState().CipherSuite, !limit)

				modelConn := Connection{
					c,
					protoConn,
				}

				s.model.AddConnection(modelConn)
				s.mut.Lock()
				s.currentConnection[remoteID] = modelConn
				s.mut.Unlock()
				continue next
			}
		}

		if !s.cfg.IgnoredDevice(remoteID) {
			events.Default.Log(events.DeviceRejected, map[string]string{
				"device":  remoteID.String(),
				"address": c.RemoteAddr().String(),
			})
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
			ct := s.currentConnection[deviceID]
			s.mut.RUnlock()

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

			for _, addr := range addrs {
				uri, err := url.Parse(addr)
				if err != nil {
					l.Infoln("Failed to parse connection url:", addr, err)
					continue
				}

				dialer, ok := dialers[uri.Scheme]
				if !ok {
					l.Infoln("Unknown address schema", uri)
					continue
				}
				if connected && dialer.Priority() < ct.Priority {
					l.Debugln("Not dialing using %s as priorty is less than current connection (%d < %d)", dialer, dialer.Priority(), ct.Priority)
					continue
				}

				l.Debugln("dial", deviceCfg.DeviceID, uri)
				conn, err := dialer.Dial(deviceID, uri, s.tlsCfg)
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

		time.Sleep(delay)
		delay *= 2
		if maxD := time.Duration(s.cfg.Options().ReconnectIntervalS) * time.Second; delay > maxD {
			delay = maxD
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

func (s *Service) AllAddresses() []string {
	var addrs []string
	for _, listener := range s.listeners {
		for _, lanAddr := range listener.LANAddresses() {
			addrs = append(addrs, lanAddr.String())
		}
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	return util.UniqueStrings(addrs)
}

func (s *Service) ExternalAddresses() []string {
	var addrs []string
	for _, listener := range s.listeners {
		for _, wanAddr := range listener.WANAddresses() {
			addrs = append(addrs, wanAddr.String())
		}
	}
	return util.UniqueStrings(addrs)
}

func (s *Service) Status() map[string]interface{} {
	result := make(map[string]interface{})
	for addr, listener := range s.listeners {
		status := make(map[string]interface{})
		status["error"] = listener.Error()
		status["details"] = listener.Details()

		result[addr] = status
	}
	return result
}

// serviceFunc wraps a function to create a suture.Service without stop
// functionality.
type serviceFunc func()

func (f serviceFunc) Serve() { f() }
func (f serviceFunc) Stop()  {}
