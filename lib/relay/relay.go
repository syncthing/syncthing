// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

import (
	"crypto/tls"
	"net"
	"net/url"
	"time"

	"github.com/syncthing/relaysrv/client"
	"github.com/syncthing/relaysrv/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture"
)

func NewSvc(cfg *config.Wrapper, tlsCfg *tls.Config, conns chan<- model.IntermediateConnection) *Svc {
	svc := &Svc{
		Supervisor: suture.New("Svc", suture.Spec{
			Log: func(log string) {
				if debug {
					l.Infoln(log)
				}
			},
			FailureBackoff:   5 * time.Minute,
			FailureDecay:     float64((10 * time.Minute) / time.Second),
			FailureThreshold: 5,
		}),
		cfg:    cfg,
		tlsCfg: tlsCfg,

		tokens:  make(map[string]suture.ServiceToken),
		clients: make(map[string]*client.ProtocolClient),
		mut:     sync.NewRWMutex(),

		invitations: make(chan protocol.SessionInvitation),
	}

	rcfg := cfg.Raw()
	svc.CommitConfiguration(rcfg, rcfg)
	cfg.Subscribe(svc)

	receiver := &invitationReceiver{
		tlsCfg:      tlsCfg,
		conns:       conns,
		invitations: svc.invitations,
	}

	svc.receiverToken = svc.Add(receiver)

	return svc
}

type Svc struct {
	*suture.Supervisor
	cfg    *config.Wrapper
	tlsCfg *tls.Config

	receiverToken suture.ServiceToken
	tokens        map[string]suture.ServiceToken
	clients       map[string]*client.ProtocolClient
	mut           sync.RWMutex
	invitations   chan protocol.SessionInvitation
}

func (s *Svc) VerifyConfiguration(from, to config.Configuration) error {
	for _, addr := range to.Options.RelayServers {
		_, err := url.Parse(addr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Svc) CommitConfiguration(from, to config.Configuration) bool {
	existing := make(map[string]struct{}, len(to.Options.RelayServers))
	for _, addr := range to.Options.RelayServers {
		uri, err := url.Parse(addr)
		if err != nil {
			if debug {
				l.Debugln("Failed to parse relay address", addr, err)
			}
			continue
		}

		existing[uri.String()] = struct{}{}

		_, ok := s.tokens[uri.String()]
		if !ok {
			if debug {
				l.Debugln("Connecting to relay", uri)
			}
			c := client.NewProtocolClient(uri, s.tlsCfg.Certificates, s.invitations)
			s.tokens[uri.String()] = s.Add(c)
			s.mut.Lock()
			s.clients[uri.String()] = c
			s.mut.Unlock()
		}
	}

	for uri, token := range s.tokens {
		_, ok := existing[uri]
		if !ok {
			err := s.Remove(token)
			delete(s.tokens, uri)
			s.mut.Lock()
			delete(s.clients, uri)
			s.mut.Unlock()
			if debug {
				l.Debugln("Disconnecting from relay", uri, err)
			}
		}
	}

	return true
}

func (s *Svc) ClientStatus() map[string]bool {
	s.mut.RLock()
	status := make(map[string]bool, len(s.clients))
	for uri, client := range s.clients {
		status[uri] = client.StatusOK()
	}
	s.mut.RUnlock()
	return status
}

type invitationReceiver struct {
	invitations chan protocol.SessionInvitation
	tlsCfg      *tls.Config
	conns       chan<- model.IntermediateConnection
	stop        chan struct{}
}

func (r *invitationReceiver) Serve() {
	if r.stop != nil {
		return
	}
	r.stop = make(chan struct{})

	for {
		select {
		case inv := <-r.invitations:
			if debug {
				l.Debugln("Received relay invitation", inv)
			}
			conn, err := client.JoinSession(inv)
			if err != nil {
				if debug {
					l.Debugf("Failed to join relay session %s: %v", inv, err)
				}
				continue
			}

			err = osutil.SetTCPOptions(conn.(*net.TCPConn))
			if err != nil {
				l.Infoln(err)
			}

			var tc *tls.Conn

			if inv.ServerSocket {
				tc = tls.Server(conn, r.tlsCfg)
			} else {
				tc = tls.Client(conn, r.tlsCfg)
			}
			err = tc.Handshake()
			if err != nil {
				l.Infof("TLS handshake (BEP/relay %s): %v", inv, err)
				tc.Close()
				continue
			}
			r.conns <- model.IntermediateConnection{
				tc, model.ConnectionTypeRelayAccept,
			}
		case <-r.stop:
			return
		}
	}
}

func (r *invitationReceiver) Stop() {
	if r.stop == nil {
		return
	}
	r.stop <- struct{}{}
	r.stop = nil
}
