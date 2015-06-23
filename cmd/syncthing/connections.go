// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/thejerf/suture"
)

type DialerFactory func(*url.URL, *tls.Config) (*tls.Conn, error)
type ListenerFactory func(*url.URL, *tls.Config, chan<- *tls.Conn)

var (
	dialers   = make(map[string]DialerFactory, 0)
	listeners = make(map[string]ListenerFactory, 0)
)

// The connection service listens on TLS and dials configured unconnected
// devices. Successful connections are handed to the model.
type connectionSvc struct {
	*suture.Supervisor
	cfg    *config.Wrapper
	myID   protocol.DeviceID
	model  *model.Model
	tlsCfg *tls.Config
	conns  chan *tls.Conn
}

func newConnectionSvc(cfg *config.Wrapper, myID protocol.DeviceID, model *model.Model, tlsCfg *tls.Config) *connectionSvc {
	svc := &connectionSvc{
		Supervisor: suture.NewSimple("connectionSvc"),
		cfg:        cfg,
		myID:       myID,
		model:      model,
		tlsCfg:     tlsCfg,
		conns:      make(chan *tls.Conn),
	}

	// There are several moving parts here; one routine per listening address
	// to handle incoming connections, one routine to periodically attempt
	// outgoing connections, and lastly one routine to the the common handling
	// regardless of whether the connection was incoming or outgoing. It ends
	// up as in the diagram below. We embed a Supervisor to manage the
	// routines (i.e. log and restart if they crash or exit, etc).
	//
	//                +-----------------+
	//    Incoming    | +---------------+-+      +-----------------+
	//   Connections  | |                 |      |                 |
	// -------------->| |    listener     |      |                 |  Outgoing connections via dialers
	//                | |  (1 per listen  |      |   svc.connect   |----------------------------------->
	//                | |    address)     |      |                 |
	//                +-+                 |      |                 |
	//                  +-----------------+      +-----------------+
	//                           v                        v
	//                           |                        |
	//                           |                        |
	//                           +------------+-----------+
	//                                        |
	//                                        | svc.conns
	//                                        v
	//                               +-----------------+
	//                               |                 |
	//                               |                 |
	//                               |   svc.handle    |------> model.AddConnection()
	//                               |                 |
	//                               |                 |
	//                               +-----------------+
	//
	// TODO: Clean shutdown, and/or handling config changes on the fly. We
	// partly do this now - new devices and addresses will be picked up, but
	// not new listen addresses and we don't support disconnecting devices
	// that are removed and so on...

	svc.Add(serviceFunc(svc.connect))
	for _, addr := range svc.cfg.Options().ListenAddress {
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

		if debugNet {
			l.Debugln("listening on", uri.String())
		}

		svc.Add(serviceFunc(func() {
			listener(uri, svc.tlsCfg, svc.conns)
		}))
	}
	svc.Add(serviceFunc(svc.handle))

	return svc
}

func (s *connectionSvc) handle() {
next:
	for conn := range s.conns {
		cs := conn.ConnectionState()

		// We should have negotiated the next level protocol "bep/1.0" as part
		// of the TLS handshake. Unfortunately this can't be a hard error,
		// because there are implementations out there that don't support
		// protocol negotiation (iOS for one...).
		if !cs.NegotiatedProtocolIsMutual || cs.NegotiatedProtocol != bepProtocolName {
			l.Infof("Peer %s did not negotiate bep/1.0", conn.RemoteAddr())
		}

		// We should have received exactly one certificate from the other
		// side. If we didn't, they don't have a device ID and we drop the
		// connection.
		certs := cs.PeerCertificates
		if cl := len(certs); cl != 1 {
			l.Infof("Got peer certificate list of length %d != 1 from %s; protocol error", cl, conn.RemoteAddr())
			conn.Close()
			continue
		}
		remoteCert := certs[0]
		remoteID := protocol.NewDeviceID(remoteCert.Raw)

		// The device ID should not be that of ourselves. It can happen
		// though, especially in the presence of NAT hairpinning, multiple
		// clients between the same NAT gateway, and global discovery.
		if remoteID == myID {
			l.Infof("Connected to myself (%s) - should not happen", remoteID)
			conn.Close()
			continue
		}

		// We should not already be connected to the other party. TODO: This
		// could use some better handling. If the old connection is dead but
		// hasn't timed out yet we may want to drop *that* connection and keep
		// this one. But in case we are two devices connecting to each other
		// in parallel we don't want to do that or we end up with no
		// connections still established...
		if s.model.ConnectedTo(remoteID) {
			l.Infof("Connected to already connected device (%s)", remoteID)
			conn.Close()
			continue
		}

		for deviceID, deviceCfg := range s.cfg.Devices() {
			if deviceID == remoteID {
				// Verify the name on the certificate. By default we set it to
				// "syncthing" when generating, but the user may have replaced
				// the certificate and used another name.
				certName := deviceCfg.CertName
				if certName == "" {
					certName = tlsDefaultCommonName
				}
				err := remoteCert.VerifyHostname(certName)
				if err != nil {
					// Incorrect certificate name is something the user most
					// likely wants to know about, since it's an advanced
					// config. Warn instead of Info.
					l.Warnf("Bad certificate from %s (%v): %v", remoteID, conn.RemoteAddr(), err)
					conn.Close()
					continue next
				}

				// If rate limiting is set, and based on the address we should
				// limit the connection, then we wrap it in a limiter.

				limit := s.shouldLimit(conn.RemoteAddr())

				wr := io.Writer(conn)
				if limit && writeRateLimit != nil {
					wr = &limitedWriter{conn, writeRateLimit}
				}

				rd := io.Reader(conn)
				if limit && readRateLimit != nil {
					rd = &limitedReader{conn, readRateLimit}
				}

				name := fmt.Sprintf("%s-%s", conn.LocalAddr(), conn.RemoteAddr())
				protoConn := protocol.NewConnection(remoteID, rd, wr, s.model, name, deviceCfg.Compression)

				l.Infof("Established secure connection to %s at %s", remoteID, name)
				if debugNet {
					l.Debugf("cipher suite: %04X in lan: %t", conn.ConnectionState().CipherSuite, !limit)
				}

				s.model.AddConnection(conn, protoConn)
				continue next
			}
		}

		if !s.cfg.IgnoredDevice(remoteID) {
			events.Default.Log(events.DeviceRejected, map[string]string{
				"device":  remoteID.String(),
				"address": conn.RemoteAddr().String(),
			})
			l.Infof("Connection from %s with unknown device ID %s", conn.RemoteAddr(), remoteID)
		} else {
			l.Infof("Connection from %s with ignored device ID %s", conn.RemoteAddr(), remoteID)
		}

		conn.Close()
	}
}

func (s *connectionSvc) connect() {
	delay := time.Second
	for {
	nextDevice:
		for deviceID, deviceCfg := range s.cfg.Devices() {
			if deviceID == myID {
				continue
			}

			if s.model.ConnectedTo(deviceID) {
				continue
			}

			var addrs []string
			for _, addr := range deviceCfg.Addresses {
				if addr == "dynamic" {
					if discoverer != nil {
						t, _ := discoverer.Lookup(deviceID)
						if len(t) == 0 {
							continue
						}
						addrs = append(addrs, t...)
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
					l.Infoln("Unknown address schema", uri.String())
					continue
				}

				if debugNet {
					l.Debugln("dial", deviceCfg.DeviceID, uri.String())
				}
				conn, err := dialer(uri, s.tlsCfg)
				if err != nil {
					if debugNet {
						l.Debugln("dial failed", deviceCfg.DeviceID, uri.String(), err)
					}
					continue
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

func (s *connectionSvc) shouldLimit(addr net.Addr) bool {
	if s.cfg.Options().LimitBandwidthInLan {
		return true
	}

	tcpaddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return true
	}
	for _, lan := range lans {
		if lan.Contains(tcpaddr.IP) {
			return false
		}
	}
	return !tcpaddr.IP.IsLoopback()
}

func (s *connectionSvc) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (s *connectionSvc) CommitConfiguration(from, to config.Configuration) bool {
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

func setTCPOptions(conn *net.TCPConn) {
	var err error
	if err = conn.SetLinger(0); err != nil {
		l.Infoln(err)
	}
	if err = conn.SetNoDelay(false); err != nil {
		l.Infoln(err)
	}
	if err = conn.SetKeepAlivePeriod(60 * time.Second); err != nil {
		l.Infoln(err)
	}
	if err = conn.SetKeepAlive(true); err != nil {
		l.Infoln(err)
	}
}
