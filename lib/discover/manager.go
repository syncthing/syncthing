// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"context"
	"crypto/tls"
	"fmt"
	"sort"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
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
}

type manager struct {
	*suture.Supervisor
	myID          protocol.DeviceID
	cfg           config.Wrapper
	cert          tls.Certificate
	evLogger      events.Logger
	addressLister AddressLister

	finders map[string]cachedFinder
	mut     sync.RWMutex
}

func NewManager(myID protocol.DeviceID, cfg config.Wrapper, cert tls.Certificate, evLogger events.Logger, lister AddressLister) Manager {
	m := &manager{
		Supervisor:    suture.New("discover.Manager", util.SpecWithDebugLogger(l)),
		myID:          myID,
		cfg:           cfg,
		cert:          cert,
		evLogger:      evLogger,
		addressLister: lister,

		finders: make(map[string]cachedFinder),
		mut:     sync.NewRWMutex(),
	}
	m.Add(util.AsService(m.serve, m.String()))
	return m
}

func (m *manager) serve(ctx context.Context) error {
	m.cfg.Subscribe(m)
	m.CommitConfiguration(config.Configuration{}, m.cfg.RawCopy())
	<-ctx.Done()
	m.cfg.Unsubscribe(m)
	return nil
}

func (m *manager) addLocked(identity string, finder Finder, cacheTime, negCacheTime time.Duration) {
	entry := cachedFinder{
		Finder:       finder,
		cacheTime:    cacheTime,
		negCacheTime: negCacheTime,
		cache:        newCache(),
		token:        nil,
	}
	if service, ok := finder.(suture.Service); ok {
		token := m.Supervisor.Add(service)
		entry.token = &token
	}
	m.finders[identity] = entry
	l.Infoln("Using discovery mechanism:", identity)
}

func (m *manager) removeLocked(identity string) {
	entry, ok := m.finders[identity]
	if !ok {
		return
	}
	if entry.token != nil {
		err := m.Supervisor.Remove(*entry.token)
		if err != nil {
			l.Warnf("removing discovery %s: %s", identity, err)
		}
	}
	delete(m.finders, identity)
	l.Infoln("Stopped using discovery mechanism: ", identity)
}

// Lookup attempts to resolve the device ID using any of the added Finders,
// while obeying the cache settings.
func (m *manager) Lookup(ctx context.Context, deviceID protocol.DeviceID) (addresses []string, err error) {
	m.mut.RLock()
	for _, finder := range m.finders {
		if cacheEntry, ok := finder.cache.Get(deviceID); ok {
			// We have a cache entry. Lets see what it says.

			if cacheEntry.found && time.Since(cacheEntry.when) < finder.cacheTime {
				// It's a positive, valid entry. Use it.
				l.Debugln("cached discovery entry for", deviceID, "at", finder)
				l.Debugln("  cache:", cacheEntry)
				addresses = append(addresses, cacheEntry.Addresses...)
				continue
			}

			valid := time.Now().Before(cacheEntry.validUntil) || time.Since(cacheEntry.when) < finder.negCacheTime
			if !cacheEntry.found && valid {
				// It's a negative, valid entry. We should not make another
				// attempt right now.
				l.Debugln("negative cache entry for", deviceID, "at", finder, "valid until", cacheEntry.when.Add(finder.negCacheTime), "or", cacheEntry.validUntil)
				continue
			}

			// It's expired. Ignore and continue.
		}

		// Perform the actual lookup and cache the result.
		if addrs, err := finder.Lookup(ctx, deviceID); err == nil {
			l.Debugln("lookup for", deviceID, "at", finder)
			l.Debugln("  addresses:", addrs)
			addresses = append(addresses, addrs...)
			finder.cache.Set(deviceID, CacheEntry{
				Addresses: addrs,
				when:      time.Now(),
				found:     len(addrs) > 0,
			})
		} else {
			// Lookup returned error, add a negative cache entry.
			entry := CacheEntry{
				when:  time.Now(),
				found: false,
			}
			if err, ok := err.(cachedError); ok {
				entry.validUntil = time.Now().Add(err.CacheFor())
			}
			finder.cache.Set(deviceID, entry)
		}
	}
	m.mut.RUnlock()

	addresses = util.UniqueTrimmedStrings(addresses)
	sort.Strings(addresses)

	l.Debugln("lookup results for", deviceID)
	l.Debugln("  addresses: ", addresses)

	return addresses, nil
}

func (m *manager) String() string {
	return "discovery cache"
}

func (m *manager) Error() error {
	return nil
}

func (m *manager) ChildErrors() map[string]error {
	children := make(map[string]error, len(m.finders))
	m.mut.RLock()
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
		v.Addresses = util.UniqueTrimmedStrings(v.Addresses)
		res[k] = v
	}

	return res
}

func (m *manager) VerifyConfiguration(_, _ config.Configuration) error {
	return nil
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
			gd, err := NewGlobal(srv, m.cert, m.addressLister, m.evLogger)
			if err != nil {
				l.Warnln("Global discovery:", err)
				continue
			}

			// Each global discovery server gets its results cached for five
			// minutes, and is not asked again for a minute when it's returned
			// unsuccessfully.
			m.addLocked(identity, gd, 5*time.Minute, time.Minute)
		}
	}

	if to.Options.LocalAnnEnabled {
		// v4 broadcasts
		v4Identity := ipv4Identity(to.Options.LocalAnnPort)
		if _, ok := m.finders[v4Identity]; !ok {
			bcd, err := NewLocal(m.myID, fmt.Sprintf(":%d", to.Options.LocalAnnPort), m.addressLister, m.evLogger)
			if err != nil {
				l.Warnln("IPv4 local discovery:", err)
			} else {
				m.addLocked(v4Identity, bcd, 0, 0)
			}
		}

		// v6 multicasts
		v6Identity := ipv6Identity(to.Options.LocalAnnMCAddr)
		if _, ok := m.finders[v6Identity]; !ok {
			mcd, err := NewLocal(m.myID, to.Options.LocalAnnMCAddr, m.addressLister, m.evLogger)
			if err != nil {
				l.Warnln("IPv6 local discovery:", err)
			} else {
				m.addLocked(v6Identity, mcd, 0, 0)
			}
		}
	}

	return true
}
