// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

import (
	"crypto/tls"

	"net/url"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture"
)

const (
	eventBroadcasterCheckInterval = 10 * time.Second
)

type Svc struct {
	*suture.Supervisor
	cfg    *config.Wrapper
	tlsCfg *tls.Config

	tokens      map[string]suture.ServiceToken
	clients     map[string]client.RelayClient
	mut         sync.RWMutex
	invitations chan protocol.SessionInvitation
	conns       chan *tls.Conn
}

func NewSvc(cfg *config.Wrapper, tlsCfg *tls.Config) *Svc {
	conns := make(chan *tls.Conn)

	svc := &Svc{
		Supervisor: suture.New("Svc", suture.Spec{
			Log: func(log string) {
				l.Debugln(log)
			},
			FailureBackoff:   5 * time.Minute,
			FailureDecay:     float64((10 * time.Minute) / time.Second),
			FailureThreshold: 5,
		}),
		cfg:    cfg,
		tlsCfg: tlsCfg,

		tokens:      make(map[string]suture.ServiceToken),
		clients:     make(map[string]client.RelayClient),
		mut:         sync.NewRWMutex(),
		invitations: make(chan protocol.SessionInvitation),
		conns:       conns,
	}

	rcfg := cfg.Raw()
	svc.CommitConfiguration(rcfg, rcfg)
	cfg.Subscribe(svc)

	receiver := &invitationReceiver{
		tlsCfg:      tlsCfg,
		conns:       conns,
		invitations: svc.invitations,
		stop:        make(chan struct{}),
	}

	eventBc := &eventBroadcaster{
		svc:  svc,
		stop: make(chan struct{}),
	}

	svc.Add(receiver)
	svc.Add(eventBc)

	return svc
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
	existing := make(map[string]*url.URL, len(to.Options.RelayServers))

	for _, addr := range to.Options.RelayServers {
		uri, err := url.Parse(addr)
		if err != nil {
			l.Debugln("Failed to parse relay address", addr, err)
			continue
		}
		existing[uri.String()] = uri
	}

	s.mut.Lock()

	for key, uri := range existing {
		_, ok := s.tokens[key]
		if !ok {
			l.Debugln("Connecting to relay", uri)
			c, err := client.NewClient(uri, s.tlsCfg.Certificates, s.invitations)
			if err != nil {
				l.Debugln("Failed to connect to relay", uri, err)
				continue
			}
			s.tokens[key] = s.Add(c)
			s.clients[key] = c
		}
	}

	for key, token := range s.tokens {
		_, ok := existing[key]
		if !ok {
			err := s.Remove(token)
			delete(s.tokens, key)
			delete(s.clients, key)
			l.Debugln("Disconnecting from relay", key, err)
		}
	}

	s.mut.Unlock()

	return true
}

type Status struct {
	URL     string
	OK      bool
	Latency int
}

// Relays return the list of relays that currently have an OK status.
func (s *Svc) Relays() []string {
	if s == nil {
		// A nil client does not have a status, really. Yet we may be called
		// this way, for raisins...
		return nil
	}

	s.mut.RLock()
	relays := make([]string, 0, len(s.clients))
	for _, client := range s.clients {
		relays = append(relays, client.URI().String())
	}
	s.mut.RUnlock()

	sort.Strings(relays)

	return relays
}

// RelayStatus returns the latency and OK status for a given relay.
func (s *Svc) RelayStatus(uri string) (time.Duration, bool) {
	if s == nil {
		// A nil client does not have a status, really. Yet we may be called
		// this way, for raisins...
		return time.Hour, false
	}

	s.mut.RLock()
	defer s.mut.RUnlock()

	for _, client := range s.clients {
		if client.URI().String() == uri {
			return client.Latency(), client.StatusOK()
		}
	}

	return time.Hour, false
}

// Accept returns a new *tls.Conn. The connection is already handshaken.
func (s *Svc) Accept() *tls.Conn {
	return <-s.conns
}

type invitationReceiver struct {
	invitations chan protocol.SessionInvitation
	tlsCfg      *tls.Config
	conns       chan<- *tls.Conn
	stop        chan struct{}
}

func (r *invitationReceiver) Serve() {
	for {
		select {
		case inv := <-r.invitations:
			l.Debugln("Received relay invitation", inv)
			conn, err := client.JoinSession(inv)
			if err != nil {
				l.Debugf("Failed to join relay session %s: %v", inv, err)
				continue
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
			r.conns <- tc

		case <-r.stop:
			return
		}
	}
}

func (r *invitationReceiver) Stop() {
	close(r.stop)
}

// The eventBroadcaster sends a RelayStateChanged event when the relay status
// changes. We need this somewhat ugly polling mechanism as there's currently
// no way to get the event feed directly from the relay lib. This may be
// something to revisit later, possibly.
type eventBroadcaster struct {
	svc  *Svc
	stop chan struct{}
}

func (e *eventBroadcaster) Serve() {
	timer := time.NewTicker(eventBroadcasterCheckInterval)
	defer timer.Stop()

	var prevOKRelays []string

	for {
		select {
		case <-timer.C:
			curOKRelays := e.svc.Relays()

			changed := len(curOKRelays) != len(prevOKRelays)
			if !changed {
				for i := range curOKRelays {
					if curOKRelays[i] != prevOKRelays[i] {
						changed = true
						break
					}
				}
			}

			if changed {
				events.Default.Log(events.RelayStateChanged, map[string][]string{
					"old": prevOKRelays,
					"new": curOKRelays,
				})
			}

			prevOKRelays = curOKRelays

		case <-e.stop:
			return
		}
	}
}

func (e *eventBroadcaster) Stop() {
	close(e.stop)
}
