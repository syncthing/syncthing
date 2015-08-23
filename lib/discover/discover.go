// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/beacon"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/relay"
	"github.com/syncthing/syncthing/lib/sync"
)

type Discoverer struct {
	myID            protocol.DeviceID
	listenAddrs     []string
	relaySvc        *relay.Svc
	localBcastIntv  time.Duration
	localBcastStart time.Time
	cacheLifetime   time.Duration
	negCacheCutoff  time.Duration
	beacons         []beacon.Interface
	extPort         uint16
	localBcastTick  <-chan time.Time
	forcedBcastTick chan time.Time

	registryLock    sync.RWMutex
	addressRegistry map[protocol.DeviceID][]CacheEntry
	relayRegistry   map[protocol.DeviceID][]CacheEntry
	lastLookup      map[protocol.DeviceID]time.Time

	clients []Client
	mut     sync.RWMutex
}

type CacheEntry struct {
	Address string
	Seen    time.Time
}

var (
	ErrIncorrectMagic = errors.New("incorrect magic number")
)

func NewDiscoverer(id protocol.DeviceID, addresses []string, relaySvc *relay.Svc) *Discoverer {
	return &Discoverer{
		myID:            id,
		listenAddrs:     addresses,
		relaySvc:        relaySvc,
		localBcastIntv:  30 * time.Second,
		cacheLifetime:   5 * time.Minute,
		negCacheCutoff:  3 * time.Minute,
		addressRegistry: make(map[protocol.DeviceID][]CacheEntry),
		relayRegistry:   make(map[protocol.DeviceID][]CacheEntry),
		lastLookup:      make(map[protocol.DeviceID]time.Time),
		registryLock:    sync.NewRWMutex(),
		mut:             sync.NewRWMutex(),
	}
}

func (d *Discoverer) StartLocal(localPort int, localMCAddr string) {
	if localPort > 0 {
		d.startLocalIPv4Broadcasts(localPort)
	}

	if len(localMCAddr) > 0 {
		d.startLocalIPv6Multicasts(localMCAddr)
	}

	if len(d.beacons) == 0 {
		l.Warnln("Local discovery unavailable")
		return
	}

	d.localBcastTick = time.Tick(d.localBcastIntv)
	d.forcedBcastTick = make(chan time.Time)
	d.localBcastStart = time.Now()
	go d.sendLocalAnnouncements()
}

func (d *Discoverer) startLocalIPv4Broadcasts(localPort int) {
	bb := beacon.NewBroadcast(localPort)
	d.beacons = append(d.beacons, bb)
	go d.recvAnnouncements(bb)
	bb.ServeBackground()
}

func (d *Discoverer) startLocalIPv6Multicasts(localMCAddr string) {
	mb, err := beacon.NewMulticast(localMCAddr)
	if err != nil {
		if debug {
			l.Debugln("beacon.NewMulticast:", err)
		}
		l.Infoln("Local discovery over IPv6 unavailable")
		return
	}
	d.beacons = append(d.beacons, mb)
	go d.recvAnnouncements(mb)
}

func (d *Discoverer) StartGlobal(servers []string, extPort uint16) {
	d.mut.Lock()
	defer d.mut.Unlock()

	if len(d.clients) > 0 {
		d.stopGlobal()
	}

	d.extPort = extPort
	wg := sync.NewWaitGroup()
	clients := make(chan Client, len(servers))
	for _, address := range servers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			client, err := New(addr, d)
			if err != nil {
				l.Infoln("Error creating discovery client", addr, err)
				return
			}
			clients <- client
		}(address)
	}

	wg.Wait()
	close(clients)

	for client := range clients {
		d.clients = append(d.clients, client)
	}
}

func (d *Discoverer) StopGlobal() {
	d.mut.Lock()
	defer d.mut.Unlock()
	d.stopGlobal()
}

func (d *Discoverer) stopGlobal() {
	for _, client := range d.clients {
		client.Stop()
	}
	d.clients = []Client{}
}

func (d *Discoverer) ExtAnnounceOK() map[string]bool {
	d.mut.RLock()
	defer d.mut.RUnlock()

	ret := make(map[string]bool)
	for _, client := range d.clients {
		ret[client.Address()] = client.StatusOK()
	}
	return ret
}

// Lookup returns a list of addresses the device is available at, as well as
// a list of relays the device is supposed to be available on sorted by the
// sum of latencies between this device, and the device in question.
func (d *Discoverer) Lookup(device protocol.DeviceID) ([]string, []string) {
	d.registryLock.RLock()
	cachedAddresses := d.filterCached(d.addressRegistry[device])
	cachedRelays := d.filterCached(d.relayRegistry[device])
	lastLookup := d.lastLookup[device]
	d.registryLock.RUnlock()

	d.mut.RLock()
	defer d.mut.RUnlock()

	relays := make([]string, len(cachedRelays))
	for i := range cachedRelays {
		relays[i] = cachedRelays[i].Address
	}

	if len(cachedAddresses) > 0 {
		// There are cached address entries.
		addrs := make([]string, len(cachedAddresses))
		for i := range cachedAddresses {
			addrs[i] = cachedAddresses[i].Address
		}
		return addrs, relays
	}

	if time.Since(lastLookup) < d.negCacheCutoff {
		// We have recently tried to lookup this address and failed. Lets
		// chill for a while.
		return nil, relays
	}

	if len(d.clients) != 0 && time.Since(d.localBcastStart) > d.localBcastIntv {
		// Only perform external lookups if we have at least one external
		// server client and one local announcement interval has passed. This is
		// to avoid finding local peers on their remote address at startup.
		results := make(chan Announce, len(d.clients))
		wg := sync.NewWaitGroup()
		for _, client := range d.clients {
			wg.Add(1)
			go func(c Client) {
				defer wg.Done()
				ann, err := c.Lookup(device)
				if err == nil {
					results <- ann
				}

			}(client)
		}

		wg.Wait()
		close(results)

		cachedAddresses := []CacheEntry{}
		availableRelays := []Relay{}
		seenAddresses := make(map[string]struct{})
		seenRelays := make(map[string]struct{})
		now := time.Now()

		var addrs []string
		for result := range results {
			for _, addr := range result.This.Addresses {
				_, ok := seenAddresses[addr]
				if !ok {
					cachedAddresses = append(cachedAddresses, CacheEntry{
						Address: addr,
						Seen:    now,
					})
					seenAddresses[addr] = struct{}{}
					addrs = append(addrs, addr)
				}
			}

			for _, relay := range result.This.Relays {
				_, ok := seenRelays[relay.Address]
				if !ok {
					availableRelays = append(availableRelays, relay)
					seenRelays[relay.Address] = struct{}{}
				}
			}
		}

		relays = addressesSortedByLatency(availableRelays)
		cachedRelays := make([]CacheEntry, len(relays))
		for i := range relays {
			cachedRelays[i] = CacheEntry{
				Address: relays[i],
				Seen:    now,
			}
		}

		d.registryLock.Lock()
		d.addressRegistry[device] = cachedAddresses
		d.relayRegistry[device] = cachedRelays
		d.lastLookup[device] = time.Now()
		d.registryLock.Unlock()

		return addrs, relays
	}

	return nil, relays
}

func (d *Discoverer) Hint(device string, addrs []string) {
	resAddrs := resolveAddrs(addrs)
	var id protocol.DeviceID
	id.UnmarshalText([]byte(device))
	d.registerDevice(nil, Device{
		Addresses: resAddrs,
		ID:        id[:],
	})
}

func (d *Discoverer) All() map[protocol.DeviceID][]CacheEntry {
	d.registryLock.RLock()
	devices := make(map[protocol.DeviceID][]CacheEntry, len(d.addressRegistry))
	for device, addrs := range d.addressRegistry {
		addrsCopy := make([]CacheEntry, len(addrs))
		copy(addrsCopy, addrs)
		devices[device] = addrsCopy
	}
	d.registryLock.RUnlock()
	return devices
}

func (d *Discoverer) Announcement() Announce {
	return d.announcementPkt(true)
}

func (d *Discoverer) announcementPkt(allowExternal bool) Announce {
	var addrs []string
	if d.extPort != 0 && allowExternal {
		addrs = []string{fmt.Sprintf("tcp://:%d", d.extPort)}
	} else {
		addrs = resolveAddrs(d.listenAddrs)
	}

	var relayAddrs []string
	if d.relaySvc != nil {
		status := d.relaySvc.ClientStatus()
		for uri, ok := range status {
			if ok {
				relayAddrs = append(relayAddrs, uri)
			}
		}
	}

	return Announce{
		Magic: AnnouncementMagic,
		This:  Device{d.myID[:], addrs, measureLatency(relayAddrs)},
	}
}

func (d *Discoverer) sendLocalAnnouncements() {
	var pkt = d.announcementPkt(false)
	msg := pkt.MustMarshalXDR()

	for {
		for _, b := range d.beacons {
			b.Send(msg)
		}

		select {
		case <-d.localBcastTick:
		case <-d.forcedBcastTick:
		}
	}
}

func (d *Discoverer) recvAnnouncements(b beacon.Interface) {
	for {
		buf, addr := b.Recv()

		var pkt Announce
		err := pkt.UnmarshalXDR(buf)
		if err != nil && err != io.EOF {
			if debug {
				l.Debugf("discover: Failed to unmarshal local announcement from %s:\n%s", addr, hex.Dump(buf))
			}
			continue
		}

		if debug {
			l.Debugf("discover: Received local announcement from %s for %s", addr, protocol.DeviceIDFromBytes(pkt.This.ID))
		}

		var newDevice bool
		if bytes.Compare(pkt.This.ID, d.myID[:]) != 0 {
			newDevice = d.registerDevice(addr, pkt.This)
		}

		if newDevice {
			select {
			case d.forcedBcastTick <- time.Now():
			}
		}
	}
}

func (d *Discoverer) registerDevice(addr net.Addr, device Device) bool {
	var id protocol.DeviceID
	copy(id[:], device.ID)

	d.registryLock.Lock()
	defer d.registryLock.Unlock()

	current := d.filterCached(d.addressRegistry[id])

	orig := current

	for _, deviceAddr := range device.Addresses {
		uri, err := url.Parse(deviceAddr)
		if err != nil {
			if debug {
				l.Debugf("discover: Failed to parse address %s: %s", deviceAddr, err)
			}
			continue
		}

		host, port, err := net.SplitHostPort(uri.Host)
		if err != nil {
			if debug {
				l.Debugf("discover: Failed to split address host %s: %s", deviceAddr, err)
			}
			continue
		}

		if host == "" {
			uri.Host = net.JoinHostPort(addr.(*net.UDPAddr).IP.String(), port)
			deviceAddr = uri.String()
		}

		for i := range current {
			if current[i].Address == deviceAddr {
				current[i].Seen = time.Now()
				goto done
			}
		}
		current = append(current, CacheEntry{
			Address: deviceAddr,
			Seen:    time.Now(),
		})
	done:
	}

	if debug {
		l.Debugf("discover: Caching %s addresses: %v", id, current)
	}

	d.addressRegistry[id] = current

	if len(current) > len(orig) {
		addrs := make([]string, len(current))
		for i := range current {
			addrs[i] = current[i].Address
		}
		events.Default.Log(events.DeviceDiscovered, map[string]interface{}{
			"device": id.String(),
			"addrs":  addrs,
		})
	}

	return len(current) > len(orig)
}

func (d *Discoverer) filterCached(c []CacheEntry) []CacheEntry {
	for i := 0; i < len(c); {
		if ago := time.Since(c[i].Seen); ago > d.cacheLifetime {
			if debug {
				l.Debugf("discover: Removing cached entry %s - seen %v ago", c[i].Address, ago)
			}
			c[i] = c[len(c)-1]
			c = c[:len(c)-1]
		} else {
			i++
		}
	}
	return c
}

func addrToAddr(addr *net.TCPAddr) string {
	if len(addr.IP) == 0 || addr.IP.IsUnspecified() {
		return fmt.Sprintf(":%d", addr.Port)
	} else if bs := addr.IP.To4(); bs != nil {
		return fmt.Sprintf("%s:%d", bs.String(), addr.Port)
	} else if bs := addr.IP.To16(); bs != nil {
		return fmt.Sprintf("[%s]:%d", bs.String(), addr.Port)
	}
	return ""
}

func resolveAddrs(addrs []string) []string {
	var raddrs []string
	for _, addrStr := range addrs {
		uri, err := url.Parse(addrStr)
		if err != nil {
			continue
		}
		addrRes, err := net.ResolveTCPAddr("tcp", uri.Host)
		if err != nil {
			continue
		}
		addr := addrToAddr(addrRes)
		if len(addr) > 0 {
			uri.Host = addr
			raddrs = append(raddrs, uri.String())
		}
	}
	return raddrs
}

func measureLatency(relayAdresses []string) []Relay {
	relays := make([]Relay, 0, len(relayAdresses))
	for i, addr := range relayAdresses {
		relay := Relay{
			Address: addr,
			Latency: int32(time.Hour / time.Millisecond),
		}
		relays = append(relays, relay)

		if latency, err := getLatencyForURL(addr); err == nil {
			if debug {
				l.Debugf("Relay %s latency %s", addr, latency)
			}
			relays[i].Latency = int32(latency / time.Millisecond)
		} else {
			l.Debugf("Failed to get relay %s latency %s", addr, err)
		}
	}
	return relays
}

// addressesSortedByLatency adds local latency to the relay, and sorts them
// by sum latency, and returns the addresses.
func addressesSortedByLatency(input []Relay) []string {
	relays := make([]Relay, len(input))
	copy(relays, input)
	for i, relay := range relays {
		if latency, err := getLatencyForURL(relay.Address); err == nil {
			relays[i].Latency += int32(latency / time.Millisecond)
		} else {
			relays[i].Latency += int32(time.Hour / time.Millisecond)
		}
	}

	sort.Sort(relayList(relays))

	addresses := make([]string, 0, len(relays))
	for _, relay := range relays {
		addresses = append(addresses, relay.Address)
	}
	return addresses
}

func getLatencyForURL(addr string) (time.Duration, error) {
	uri, err := url.Parse(addr)
	if err != nil {
		return 0, err
	}

	return osutil.TCPPing(uri.Host)
}

type relayList []Relay

func (l relayList) Len() int {
	return len(l)
}

func (l relayList) Less(a, b int) bool {
	return l[a].Latency < l[b].Latency
}

func (l relayList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
