// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package discover

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/beacon"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/protocol"
)

type Discoverer struct {
	myID             protocol.DeviceID
	listenAddrs      []string
	localBcastIntv   time.Duration
	localBcastStart  time.Time
	globalBcastIntv  time.Duration
	errorRetryIntv   time.Duration
	cacheLifetime    time.Duration
	broadcastBeacon  beacon.Interface
	multicastBeacon  beacon.Interface
	registry         map[protocol.DeviceID][]CacheEntry
	registryLock     sync.RWMutex
	extServers       []string
	extPort          uint16
	localBcastTick   <-chan time.Time
	stopGlobal       chan struct{}
	globalWG         sync.WaitGroup
	forcedBcastTick  chan time.Time
	extAnnounceOK    map[string]bool
	extAnnounceOKmut sync.Mutex
}

type CacheEntry struct {
	Address string
	Seen    time.Time
}

var (
	ErrIncorrectMagic = errors.New("incorrect magic number")
)

func NewDiscoverer(id protocol.DeviceID, addresses []string) *Discoverer {
	return &Discoverer{
		myID:            id,
		listenAddrs:     addresses,
		localBcastIntv:  30 * time.Second,
		globalBcastIntv: 1800 * time.Second,
		errorRetryIntv:  60 * time.Second,
		cacheLifetime:   5 * time.Minute,
		registry:        make(map[protocol.DeviceID][]CacheEntry),
		extAnnounceOK:   make(map[string]bool),
	}
}

func (d *Discoverer) StartLocal(localPort int, localMCAddr string) {
	if localPort > 0 {
		bb, err := beacon.NewBroadcast(localPort)
		if err != nil {
			if debug {
				l.Debugln(err)
			}
			l.Infoln("Local discovery over IPv4 unavailable")
		} else {
			d.broadcastBeacon = bb
			go d.recvAnnouncements(bb)
		}
	}

	if len(localMCAddr) > 0 {
		mb, err := beacon.NewMulticast(localMCAddr)
		if err != nil {
			if debug {
				l.Debugln(err)
			}
			l.Infoln("Local discovery over IPv6 unavailable")
		} else {
			d.multicastBeacon = mb
			go d.recvAnnouncements(mb)
		}
	}

	if d.broadcastBeacon == nil && d.multicastBeacon == nil {
		l.Warnln("Local discovery unavailable")
	} else {
		d.localBcastTick = time.Tick(d.localBcastIntv)
		d.forcedBcastTick = make(chan time.Time)
		d.localBcastStart = time.Now()
		go d.sendLocalAnnouncements()
	}
}

func (d *Discoverer) StartGlobal(servers []string, extPort uint16) {
	// Wait for any previous announcer to stop before starting a new one.
	d.globalWG.Wait()
	d.extServers = servers
	d.extPort = extPort
	d.stopGlobal = make(chan struct{})
	d.globalWG.Add(1)
	go func() {
		defer d.globalWG.Done()

		buf := d.announcementPkt()

		for _, extServer := range d.extServers {
			d.globalWG.Add(1)
			go func(server string) {
				d.sendExternalAnnouncements(server, buf)
				d.globalWG.Done()
			}(extServer)
		}
	}()
}

func (d *Discoverer) StopGlobal() {
	if d.stopGlobal != nil {
		close(d.stopGlobal)
		d.globalWG.Wait()
	}
}

func (d *Discoverer) ExtAnnounceOK() map[string]bool {
	d.extAnnounceOKmut.Lock()
	defer d.extAnnounceOKmut.Unlock()
	return d.extAnnounceOK
}

func (d *Discoverer) Lookup(device protocol.DeviceID) []string {
	d.registryLock.RLock()
	cached := d.filterCached(d.registry[device])
	d.registryLock.RUnlock()

	if len(cached) > 0 {
		addrs := make([]string, len(cached))
		for i := range cached {
			addrs[i] = cached[i].Address
		}
		return addrs
	} else if len(d.extServers) != 0 && time.Since(d.localBcastStart) > d.localBcastIntv {
		// Only perform external lookups if we have at least one external
		// server and one local announcement interval has passed. This is to
		// avoid finding local peers on their remote address at startup.
		addrs := d.externalLookup(device)
		cached = make([]CacheEntry, len(addrs))
		for i := range addrs {
			cached[i] = CacheEntry{
				Address: addrs[i],
				Seen:    time.Now(),
			}
		}

		d.registryLock.Lock()
		d.registry[device] = cached
		d.registryLock.Unlock()
	}
	return nil
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
	devices := make(map[protocol.DeviceID][]CacheEntry, len(d.registry))
	for device, addrs := range d.registry {
		addrsCopy := make([]CacheEntry, len(addrs))
		copy(addrsCopy, addrs)
		devices[device] = addrsCopy
	}
	d.registryLock.RUnlock()
	return devices
}

func (d *Discoverer) announcementPkt() []byte {
	var addrs []Address
	if d.extPort != 0 {
		addrs = []Address{{Port: d.extPort}}
	} else {
		for _, astr := range d.listenAddrs {
			addr, err := net.ResolveTCPAddr("tcp", astr)
			if err != nil {
				l.Warnln("%v: not announcing %s", err, astr)
				continue
			} else if debug {
				l.Debugf("discover: announcing %s: %#v", astr, addr)
			}
			if len(addr.IP) == 0 || addr.IP.IsUnspecified() {
				addrs = append(addrs, Address{Port: uint16(addr.Port)})
			} else if bs := addr.IP.To4(); bs != nil {
				addrs = append(addrs, Address{IP: bs, Port: uint16(addr.Port)})
			} else if bs := addr.IP.To16(); bs != nil {
				addrs = append(addrs, Address{IP: bs, Port: uint16(addr.Port)})
			}
		}
	}
	var pkt = Announce{
		Magic: AnnouncementMagic,
		This:  Device{d.myID[:], addrs},
	}
	return pkt.MustMarshalXDR()
}

func (d *Discoverer) sendLocalAnnouncements() {
	var addrs = resolveAddrs(d.listenAddrs)

	var pkt = Announce{
		Magic: AnnouncementMagic,
		This:  Device{d.myID[:], addrs},
	}
	msg := pkt.MustMarshalXDR()

	for {
		if d.multicastBeacon != nil {
			d.multicastBeacon.Send(msg)
		}
		if d.broadcastBeacon != nil {
			d.broadcastBeacon.Send(msg)
		}

		select {
		case <-d.localBcastTick:
		case <-d.forcedBcastTick:
		}
	}
}

func (d *Discoverer) sendExternalAnnouncements(extServer string, buf []byte) {
	timer := time.NewTimer(0)

	conn, err := net.ListenUDP("udp", nil)
	for err != nil {
		timer.Reset(d.errorRetryIntv)
		l.Warnf("Global discovery: %v; trying again in %v", err, d.errorRetryIntv)
		select {
		case <-d.stopGlobal:
			return
		case <-timer.C:
		}
		conn, err = net.ListenUDP("udp", nil)
	}

	remote, err := net.ResolveUDPAddr("udp", extServer)
	for err != nil {
		timer.Reset(d.errorRetryIntv)
		l.Warnf("Global discovery: %s: %v; trying again in %v", extServer, err, d.errorRetryIntv)
		select {
		case <-d.stopGlobal:
			return
		case <-timer.C:
		}
		remote, err = net.ResolveUDPAddr("udp", extServer)
	}

	// Delay the first announcement until after a full local announcement
	// cycle, to increase the chance of other peers finding us locally first.
	timer.Reset(d.localBcastIntv)

	for {
		select {
		case <-d.stopGlobal:
			return

		case <-timer.C:
			var ok bool

			if debug {
				l.Debugf("discover: send announcement -> %v\n%s", remote, hex.Dump(buf))
			}

			_, err := conn.WriteTo(buf, remote)
			if err != nil {
				if debug {
					l.Debugln("discover: %s: warning:", extServer, err)
				}
				ok = false
			} else {
				// Verify that the announce server responds positively for our device ID

				time.Sleep(1 * time.Second)
				res := d.externalLookupOnServer(extServer, d.myID)

				if debug {
					l.Debugln("discover:", extServer, "external lookup check:", res)
				}
				ok = len(res) > 0
			}

			d.extAnnounceOKmut.Lock()
			d.extAnnounceOK[extServer] = ok
			d.extAnnounceOKmut.Unlock()

			if ok {
				timer.Reset(d.globalBcastIntv)
			} else {
				timer.Reset(d.errorRetryIntv)
			}
		}
	}
}

func (d *Discoverer) recvAnnouncements(b beacon.Interface) {
	for {
		buf, addr := b.Recv()

		if debug {
			l.Debugf("discover: read announcement from %s:\n%s", addr, hex.Dump(buf))
		}

		var pkt Announce
		err := pkt.UnmarshalXDR(buf)
		if err != nil && err != io.EOF {
			continue
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

	d.registryLock.RLock()
	current := d.filterCached(d.registry[id])
	d.registryLock.RUnlock()

	orig := current

	for _, a := range device.Addresses {
		var deviceAddr string
		if len(a.IP) > 0 {
			deviceAddr = net.JoinHostPort(net.IP(a.IP).String(), strconv.Itoa(int(a.Port)))
		} else if addr != nil {
			ua := addr.(*net.UDPAddr)
			ua.Port = int(a.Port)
			deviceAddr = ua.String()
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
		l.Debugf("discover: register: %v -> %v", id, current)
	}

	d.registryLock.Lock()
	d.registry[id] = current
	d.registryLock.Unlock()

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

func (d *Discoverer) externalLookup(device protocol.DeviceID) []string {
	// Buffer up to as many answers as we have servers to query.
	results := make(chan []string, len(d.extServers))

	// Query all servers.
	wg := sync.WaitGroup{}
	for _, extServer := range d.extServers {
		wg.Add(1)
		go func(server string) {
			result := d.externalLookupOnServer(server, device)
			if debug {
				l.Debugln("discover:", result, "from", server, "for", device)
			}
			results <- result
			wg.Done()
		}(extServer)
	}

	wg.Wait()
	close(results)

	addrs := []string{}
	for result := range results {
		addrs = append(addrs, result...)
	}

	return addrs
}

func (d *Discoverer) externalLookupOnServer(extServer string, device protocol.DeviceID) []string {
	extIP, err := net.ResolveUDPAddr("udp", extServer)
	if err != nil {
		if debug {
			l.Debugf("discover: %s: %v; no external lookup", extServer, err)
		}
		return nil
	}

	conn, err := net.DialUDP("udp", nil, extIP)
	if err != nil {
		if debug {
			l.Debugf("discover: %s: %v; no external lookup", extServer, err)
		}
		return nil
	}
	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		if debug {
			l.Debugf("discover: %s: %v; no external lookup", extServer, err)
		}
		return nil
	}

	buf := Query{QueryMagic, device[:]}.MustMarshalXDR()
	_, err = conn.Write(buf)
	if err != nil {
		if debug {
			l.Debugf("discover: %s: %v; no external lookup", extServer, err)
		}
		return nil
	}

	buf = make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			// Expected if the server doesn't know about requested device ID
			return nil
		}
		if debug {
			l.Debugf("discover: %s: %v; no external lookup", extServer, err)
		}
		return nil
	}

	if debug {
		l.Debugf("discover: %s: read external:\n%s", extServer, hex.Dump(buf[:n]))
	}

	var pkt Announce
	err = pkt.UnmarshalXDR(buf[:n])
	if err != nil && err != io.EOF {
		if debug {
			l.Debugln("discover:", extServer, err)
		}
		return nil
	}

	var addrs []string
	for _, a := range pkt.This.Addresses {
		deviceAddr := net.JoinHostPort(net.IP(a.IP).String(), strconv.Itoa(int(a.Port)))
		addrs = append(addrs, deviceAddr)
	}
	return addrs
}

func (d *Discoverer) filterCached(c []CacheEntry) []CacheEntry {
	for i := 0; i < len(c); {
		if ago := time.Since(c[i].Seen); ago > d.cacheLifetime {
			if debug {
				l.Debugf("removing cached address %s: seen %v ago", c[i].Address, ago)
			}
			c[i] = c[len(c)-1]
			c = c[:len(c)-1]
		} else {
			i++
		}
	}
	return c
}

func addrToAddr(addr *net.TCPAddr) Address {
	if len(addr.IP) == 0 || addr.IP.IsUnspecified() {
		return Address{Port: uint16(addr.Port)}
	} else if bs := addr.IP.To4(); bs != nil {
		return Address{IP: bs, Port: uint16(addr.Port)}
	} else if bs := addr.IP.To16(); bs != nil {
		return Address{IP: bs, Port: uint16(addr.Port)}
	}
	return Address{}
}

func resolveAddrs(addrs []string) []Address {
	var raddrs []Address
	for _, addrStr := range addrs {
		addrRes, err := net.ResolveTCPAddr("tcp", addrStr)
		if err != nil {
			continue
		}
		addr := addrToAddr(addrRes)
		if len(addr.IP) > 0 {
			raddrs = append(raddrs, addr)
		} else {
			raddrs = append(raddrs, Address{Port: addr.Port})
		}
	}
	return raddrs
}
