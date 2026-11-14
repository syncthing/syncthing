// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Observe rather than initiate recovery: the DHT owns its bootstrap and refresh
// loops. In particular, diagnostic sampling must not reset dial backoff.
func (s *DHT) startDiagnostics(bootstrap []peer.AddrInfo) {
	ctx, cancel := context.WithCancel(context.Background())
	s.stopDiagnostics = cancel
	s.diagnosticsDone = make(chan struct{})
	sub, err := s.host.EventBus().Subscribe([]interface{}{
		new(event.EvtPeerIdentificationCompleted), new(event.EvtPeerIdentificationFailed),
		new(event.EvtLocalAddressesUpdated), new(event.EvtLocalReachabilityChanged),
	})
	if err != nil {
		slog.Warn("Failed to subscribe to DHT diagnostics", "error", err)
	}
	var events <-chan interface{}
	if sub != nil {
		events = sub.Out()
	}
	notify := &network.NotifyBundle{
		ConnectedF:    func(_ network.Network, c network.Conn) { logConnection("DHT connection opened", c) },
		DisconnectedF: func(_ network.Network, c network.Conn) { logConnection("DHT connection closed", c) },
	}
	s.host.Network().Notify(notify)
	slog.Info("DHT started", "peer", s.ID(), "mode", s.Mode(), "listen", s.host.Network().ListenAddresses(), "advertised", s.host.Addrs(), "bootstrap", bootstrap)
	go func() {
		defer close(s.diagnosticsDone)
		defer s.host.Network().StopNotify(notify)
		if sub != nil {
			defer sub.Close()
		}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		previous := -1
		var previousPeers []peer.ID
		var emptySince time.Time
		sample := func() {
			now := time.Now()
			routingPeers := s.ipfs.RoutingTable().ListPeers()
			peers := len(routingPeers)
			for _, p := range routingPeers {
				if !slices.Contains(previousPeers, p) {
					slog.Debug("DHT routing peer added", "peer", p, "addresses", s.host.Peerstore().Addrs(p))
				}
			}
			for _, p := range previousPeers {
				if !slices.Contains(routingPeers, p) {
					slog.Debug("DHT routing peer removed", "peer", p, "connectedness", s.host.Network().Connectedness(p), "addresses", s.host.Peerstore().Addrs(p))
				}
			}
			previousPeers = routingPeers
			connected := len(s.host.Network().Peers())
			known := 0
			for _, p := range s.host.Peerstore().Peers() {
				if p != s.host.ID() && len(s.host.Peerstore().Addrs(p)) > 0 {
					known++
				}
			}
			metricPeers.WithLabelValues("routing").Set(float64(peers))
			metricPeers.WithLabelValues("connected").Set(float64(connected))
			metricPeers.WithLabelValues("known_with_addresses").Set(float64(known))
			if peers == 0 && emptySince.IsZero() {
				emptySince = now
			}
			if peers == 0 {
				metricEmptyDuration.Set(now.Sub(emptySince).Seconds())
				if previous > 0 {
					metricRoutingTransitions.WithLabelValues("empty").Inc()
					slog.Warn("DHT routing table became empty", "previousPeers", previous, "connectedPeers", connected, "knownPeers", known, "mode", s.Mode())
				}
			} else {
				metricEmptyDuration.Set(0)
				metricLastRoutingSuccess.SetToCurrentTime()
				if !emptySince.IsZero() {
					metricRoutingTransitions.WithLabelValues("recovered").Inc()
					slog.Info("DHT routing table recovered", "peers", peers, "outage", now.Sub(emptySince))
					emptySince = time.Time{}
				}
			}
			slog.Debug("DHT health", "mode", s.Mode(), "routingPeers", peers, "connectedPeers", connected, "knownPeers", known, "emptySince", emptySince, "advertised", s.host.Addrs())
			previous = peers
		}
		sample()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sample()
			case ev, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				switch ev := ev.(type) {
				case event.EvtPeerIdentificationCompleted:
					slog.Debug("DHT peer identified", "peer", ev.Peer, "addresses", ev.ListenAddrs, "supportsDHT", slices.Contains(ev.Protocols, p2pProtocolID), "agent", ev.AgentVersion)
				case event.EvtPeerIdentificationFailed:
					slog.Debug("DHT peer identification failed", "peer", ev.Peer, "error", ev.Reason)
				case event.EvtLocalAddressesUpdated:
					slog.Info("DHT advertised addresses changed", "current", ev.Current, "removed", ev.Removed)
				case event.EvtLocalReachabilityChanged:
					slog.Info("DHT reachability changed", "reachability", ev.Reachability, "mode", s.Mode())
				}
			}
		}
	}()
}

func logConnection(message string, c network.Conn) {
	slog.Debug(message, "peer", c.RemotePeer(), "local", c.LocalMultiaddr(), "remote", c.RemoteMultiaddr(), "direction", c.Stat().Direction, "opened", c.Stat().Opened)
}

func observeOperation(ctx context.Context, operation string, start time.Time, err error) {
	result := "success"
	switch {
	case errors.Is(err, ErrP2PNotFound):
		result = "not_found"
	case errors.Is(err, context.DeadlineExceeded):
		result = "timeout"
	case errors.Is(err, context.Canceled):
		result = "canceled"
	case err != nil:
		result = "error"
	}
	metricOperations.WithLabelValues(operation, result).Inc()
	metricOperationDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
	if err == nil {
		metricLastSuccess.WithLabelValues(operation).SetToCurrentTime()
	}
	slog.DebugContext(ctx, "DHT operation completed", "operation", operation, "result", result, "duration", time.Since(start), "error", err)
}
