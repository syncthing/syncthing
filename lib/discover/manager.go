// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate go tool counterfeiter -o mocks/manager.go --fake-name Manager . Manager

package discover

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/stringutil"
	"github.com/syncthing/syncthing/lib/svcutil"
)

// The Manager aggregates results from multiple Finders. Each Finder has
// an associated cache time and negative cache time. The cache time sets how
// long we cache and return successful lookup results, the negative cache
// time sets how long we refrain from asking about the same device ID after
// receiving a negative answer. The value of zero disables caching (positive
// or negative).
type Manager interface {
	FinderService
	ChildErrors() map[string]error
	P2PStats() P2PStats
}

type P2PStats struct {
	Mode  string
	Peers int
}

const (
	globalCacheTime    = 15 * time.Minute
	globalNegCacheTime = time.Minute // usually overridden by the servers

	p2pCacheTime    = 15 * time.Minute
	p2pNegCacheTime = 5 * time.Minute
)

type manager struct {
	*suture.Supervisor

	myID          protocol.DeviceID
	cfg           config.Wrapper
	cert          tls.Certificate
	evLogger      events.Logger
	addressLister AddressLister
	registry      *registry.Registry
	kv            db.KV

	mut     sync.RWMutex
	finders map[string]cachedFinder
	tokens  map[string]suture.ServiceToken
}

func NewManager(myID protocol.DeviceID, cfg config.Wrapper, cert tls.Certificate, evLogger events.Logger, lister AddressLister, registry *registry.Registry, kv db.KV) Manager {
	m := &manager{
		Supervisor:    suture.New("discover.Manager", svcutil.SpecWithDebugLogger()),
		myID:          myID,
		cfg:           cfg,
		cert:          cert,
		evLogger:      evLogger,
		addressLister: lister,
		registry:      registry,
		kv:            kv,
		finders:       make(map[string]cachedFinder),
		tokens:        make(map[string]suture.ServiceToken),
	}
	m.Add(svcutil.AsService(m.serve, m.String()))
	return m
}

func (m *manager) serve(ctx context.Context) error {
	m.cfg.Subscribe(m)
	m.CommitConfiguration(config.Configuration{}, m.cfg.RawCopy())
	defer m.cfg.Unsubscribe(m)
	<-ctx.Done()
	return nil
}

func (m *manager) addLocked(identity string, finder Finder, cacheTime, negCacheTime time.Duration) {
	m.finders[identity] = cachedFinder{
		Finder:       finder,
		cacheTime:    cacheTime,
		negCacheTime: negCacheTime,
		cache:        newCache(),
	}
	if service, ok := finder.(suture.Service); ok {
		token := m.Supervisor.Add(service)
		m.tokens[identity] = token
	}
	slog.Info("Using discovery mechanism", "identity", identity)
}

func (m *manager) removeLocked(identity string) {
	if token, ok := m.tokens[identity]; ok {
		if err := m.Supervisor.Remove(token); err != nil {
			slog.Warn("Failed to remove discovery mechanism", slog.String("identity", identity), slogutil.Error(err))
		}
	}
	delete(m.finders, identity)
	delete(m.tokens, identity)
	slog.Info("Stopped using discovery mechanism", "identity", identity)
}

// Lookup attempts to resolve the device ID using any of the added Finders,
// while obeying the cache settings.
func (m *manager) Lookup(ctx context.Context, deviceID protocol.DeviceID) (addresses []string, err error) {
	m.mut.RLock()
	for _, finder := range m.finders {
		if addrs, err := finder.Lookup(ctx, deviceID); err == nil {
			addresses = append(addresses, addrs...)
		}
	}
	m.mut.RUnlock()

	addresses = stringutil.UniqueTrimmedStrings(addresses)
	slices.Sort(addresses)

	slog.DebugContext(ctx, "Final lookup results", "device", deviceID, "addresses", addresses)

	return addresses, nil
}

func (*manager) String() string {
	return "discovery cache"
}

func (*manager) Error() error {
	return nil
}

func (m *manager) ChildErrors() map[string]error {
	m.mut.RLock()
	children := make(map[string]error, len(m.finders))
	for _, f := range m.finders {
		children[f.String()] = f.Error()
	}
	m.mut.RUnlock()
	return children
}

func (m *manager) Cache() map[protocol.DeviceID]CacheEntry {
	// Res will be the "total" cache, i.e. the union of our cache and all our
	// children's caches.
	res := make(map[protocol.DeviceID]CacheEntry)

	m.mut.RLock()
	for _, finder := range m.finders {
		// Each finder[i] has a corresponding cache. Go through
		// it and populate the total, appending any addresses and keeping
		// the newest "when" time. We skip any negative cache finders.
		for k, v := range finder.cache.Cache() {
			if v.found {
				cur := res[k]
				if v.when.After(cur.when) {
					cur.when = v.when
				}
				cur.Addresses = append(cur.Addresses, v.Addresses...)
				res[k] = cur
			}
		}

		// Then ask the finder itself for its cache and do the same. If this
		// finder is a global discovery client, it will have no cache. If it's
		// a local discovery client, this will be its current state.
		for k, v := range finder.Cache() {
			if v.found {
				cur := res[k]
				if v.when.After(cur.when) {
					cur.when = v.when
				}
				cur.Addresses = append(cur.Addresses, v.Addresses...)
				res[k] = cur
			}
		}
	}
	m.mut.RUnlock()

	for k, v := range res {
		v.Addresses = stringutil.UniqueTrimmedStrings(v.Addresses)
		res[k] = v
	}

	return res
}

func (m *manager) CommitConfiguration(_, to config.Configuration) (handled bool) {
	m.mut.Lock()
	defer m.mut.Unlock()
	toIdentities := make(map[string]struct{})
	if to.Options.GlobalAnnEnabled {
		for _, srv := range to.Options.GlobalDiscoveryServers() {
			toIdentities[globalDiscoveryIdentity(srv)] = struct{}{}
		}
	}

	if to.Options.LocalAnnEnabled {
		toIdentities[ipv4Identity(to.Options.LocalAnnPort)] = struct{}{}
		toIdentities[ipv6Identity(to.Options.LocalAnnMCAddr)] = struct{}{}
	}

	if to.Options.P2PDiscoveryEnabled {
		toIdentities[p2pIdentity()] = struct{}{}
	}

	// Remove things that we're not expected to have.
	for identity := range m.finders {
		if _, ok := toIdentities[identity]; !ok {
			m.removeLocked(identity)
		}
	}

	// Add things we don't have.
	if to.Options.GlobalAnnEnabled {
		for _, srv := range to.Options.GlobalDiscoveryServers() {
			identity := globalDiscoveryIdentity(srv)
			// Skip, if it's already running.
			if _, ok := m.finders[identity]; ok {
				continue
			}
			gd, err := NewGlobal(srv, m.cert, m.addressLister, m.evLogger, m.registry)
			if err != nil {
				slog.Warn("Failed to initialize global discovery", slogutil.Error(err))
				continue
			}
			m.addLocked(identity, gd, globalCacheTime, globalNegCacheTime)
		}
	}

	if to.Options.LocalAnnEnabled {
		// v4 broadcasts
		v4Identity := ipv4Identity(to.Options.LocalAnnPort)
		if _, ok := m.finders[v4Identity]; !ok {
			bcd, err := NewLocal(m.myID, fmt.Sprintf(":%d", to.Options.LocalAnnPort), m.addressLister, m.evLogger)
			if err != nil {
				slog.Warn("Failed to initialize IPv4 local discovery", slogutil.Error(err))
			} else {
				m.addLocked(v4Identity, bcd, 0, 0)
			}
		}

		// v6 multicasts
		v6Identity := ipv6Identity(to.Options.LocalAnnMCAddr)
		if _, ok := m.finders[v6Identity]; !ok {
			mcd, err := NewLocal(m.myID, to.Options.LocalAnnMCAddr, m.addressLister, m.evLogger)
			if err != nil {
				slog.Warn("Failed to initialize IPv6 local discovery", slogutil.Error(err))
			} else {
				m.addLocked(v6Identity, mcd, 0, 0)
			}
		}
	}

	if to.Options.P2PDiscoveryEnabled {
		ident := p2pIdentity()
		if _, ok := m.finders[ident]; !ok {
			listenAddresses := p2pQUICListenAddresses(to.Options.ListenAddresses())
			bootstrapPeers := to.Options.P2PDiscoveryBootstrapPeers()
			dht, err := NewPeerToPeerDiscovery(m.cert, m.addressLister, m.evLogger, m.kv, m.registry.UDP, listenAddresses, bootstrapPeers)
			if err != nil {
				slog.Warn("Failed to initialize peer-to-peer discovery", slogutil.Error(err))
			} else {
				m.addLocked(ident, dht, p2pCacheTime, p2pNegCacheTime)
			}
		}
	}

	return true
}

// P2PStats returns statistics about the currently running peer-to-peer
// discovery.
func (m *manager) P2PStats() P2PStats {
	m.mut.RLock()
	defer m.mut.RUnlock()

	p2p, ok := m.finders[p2pIdentity()].Finder.(*PeerToPeerDiscovery)
	if !ok {
		return P2PStats{}
	}

	return P2PStats{
		Mode:  p2p.Mode(),
		Peers: p2p.Peers(),
	}
}
