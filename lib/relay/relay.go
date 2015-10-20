// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
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
	clients     map[string]*client.ProtocolClient
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
		clients:     make(map[string]*client.ProtocolClient),
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

	// Query dynamic addresses, and pick the closest relay from the ones they provide.
	for key, uri := range existing {
		if uri.Scheme != "dynamic+http" && uri.Scheme != "dynamic+https" {
			continue
		}
		delete(existing, key)

		// Trim off the `dynamic+` prefix
		uri.Scheme = uri.Scheme[8:]

		l.Debugln("Looking up dynamic relays from", uri)

		data, err := http.Get(uri.String())
		if err != nil {
			l.Debugln("Failed to lookup dynamic relays", err)
			continue
		}

		var ann dynamicAnnouncement
		err = json.NewDecoder(data.Body).Decode(&ann)
		data.Body.Close()
		if err != nil {
			l.Debugln("Failed to lookup dynamic relays", err)
			continue
		}

		var dynRelayAddrs []string
		for _, relayAnn := range ann.Relays {
			ruri, err := url.Parse(relayAnn.URL)
			if err != nil {
				l.Debugln("Failed to parse dynamic relay address", relayAnn.URL, err)
				continue
			}
			l.Debugln("Found", ruri, "via", uri)
			dynRelayAddrs = append(dynRelayAddrs, ruri.String())
		}

		if len(dynRelayAddrs) > 0 {
			dynRelayAddrs = relayAddressesSortedByLatency(dynRelayAddrs)
			closestRelay := dynRelayAddrs[0]
			l.Debugln("Picking", closestRelay, "as closest dynamic relay from", uri)
			ruri, _ := url.Parse(closestRelay)
			existing[closestRelay] = ruri
		} else {
			l.Debugln("No dynamic relay found on", uri)
		}
	}

	s.mut.Lock()

	for key, uri := range existing {
		_, ok := s.tokens[key]
		if !ok {
			l.Debugln("Connecting to relay", uri)
			c := client.NewProtocolClient(uri, s.tlsCfg.Certificates, s.invitations)
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
	for uri := range s.clients {
		relays = append(relays, uri)
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
	client, ok := s.clients[uri]
	s.mut.RUnlock()

	if !ok || !client.StatusOK() {
		return time.Hour, false
	}

	return client.Latency(), true
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
// somethign to revisit later, possibly.
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

// This is the announcement recieved from the relay server;
// {"relays": [{"url": "relay://10.20.30.40:5060"}, ...]}
type dynamicAnnouncement struct {
	Relays []struct {
		URL string
	}
}

// relayAddressesSortedByLatency adds local latency to the relay, and sorts them
// by sum latency, and returns the addresses.
func relayAddressesSortedByLatency(input []string) []string {
	relays := make(relayList, len(input))
	for i, relay := range input {
		if latency, err := osutil.GetLatencyForURL(relay); err == nil {
			relays[i] = relayWithLatency{relay, int(latency / time.Millisecond)}
		} else {
			relays[i] = relayWithLatency{relay, int(time.Hour / time.Millisecond)}
		}
	}

	sort.Sort(relays)

	addresses := make([]string, len(relays))
	for i, relay := range relays {
		addresses[i] = relay.relay
	}
	return addresses
}

type relayWithLatency struct {
	relay   string
	latency int
}

type relayList []relayWithLatency

func (l relayList) Len() int {
	return len(l)
}

func (l relayList) Less(a, b int) bool {
	return l[a].latency < l[b].latency
}

func (l relayList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
