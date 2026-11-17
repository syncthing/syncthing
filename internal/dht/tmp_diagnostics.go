// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/gologshim"
	"github.com/syncthing/syncthing/internal/slogutil"
	"go.uber.org/zap/zapcore"
)

func init() { slogutil.RegisterPackage("Peer-to-peer discovery DHT") }

var setupLibraryLoggingOnce sync.Once

func setupLibraryLogging() {
	setupLibraryLoggingOnce.Do(func() {
		// Include nested swarm dial errors and DHT bootstrap/refresh diagnostics in
		// the normal Syncthing log stream and support bundles. The slog handler
		// still controls whether debug records are emitted.
		logging.SetPrimaryCore(libraryLogCore{})
		gologshim.SetDefaultHandler(logging.SlogHandler())
		for _, system := range []string{"dht", "dht/RtRefreshManager", "swarm2", "autonat"} {
			_ = logging.SetLogLevel(system, "debug")
		}
	})
}

type libraryLogCore struct{ fields []zapcore.Field }

func (c libraryLogCore) Enabled(level zapcore.Level) bool {
	return slog.Default().Enabled(context.Background(), libraryLogLevel(level))
}

func (c libraryLogCore) With(fields []zapcore.Field) zapcore.Core {
	c.fields = append(append([]zapcore.Field(nil), c.fields...), fields...)
	return c
}

func (c libraryLogCore) Check(e zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return checked.AddCore(e, c)
	}
	return checked
}

func (c libraryLogCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.fields {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}
	attrs := []slog.Attr{slog.String("libp2pSystem", e.LoggerName)}
	for k, v := range enc.Fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	slog.LogAttrs(context.Background(), libraryLogLevel(e.Level), e.Message, attrs...)
	return nil
}
func (libraryLogCore) Sync() error { return nil }
func libraryLogLevel(level zapcore.Level) slog.Level {
	switch {
	case level < zapcore.InfoLevel:
		return slog.LevelDebug
	case level == zapcore.InfoLevel:
		return slog.LevelDebug
	case level == zapcore.WarnLevel:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

func (s *DHT) startDiagnostics(bootstrap []peer.AddrInfo) {
	ctx, cancel := context.WithCancel(context.Background())
	s.stopDiagnostics = cancel
	s.diagnosticsDone = make(chan struct{})
	sub, err := s.host.EventBus().Subscribe([]interface{}{
		new(event.EvtPeerIdentificationCompleted), new(event.EvtPeerIdentificationFailed),
		new(event.EvtLocalAddressesUpdated), new(event.EvtLocalReachabilityChanged),
		new(event.EvtPeerProtocolsUpdated),
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
			if peers == 0 && emptySince.IsZero() {
				emptySince = now
			}
			if peers == 0 {
				if previous > 0 {
					slog.Warn("DHT routing table became empty", "previousPeers", previous, "connectedPeers", connected, "knownPeers", known, "mode", s.Mode())
				}
			} else {
				if !emptySince.IsZero() {
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
				case event.EvtPeerProtocolsUpdated:
					slog.Debug("DHT peer protocols changed", "peer", ev.Peer, "added", ev.Added, "removed", ev.Removed)
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
