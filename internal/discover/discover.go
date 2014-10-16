// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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
	globalBcastIntv  time.Duration
	errorRetryIntv   time.Duration
	cacheLifetime    time.Duration
	broadcastBeacon  beacon.Interface
	multicastBeacon  beacon.Interface
	registry         map[protocol.DeviceID][]CacheEntry
	registryLock     sync.RWMutex
	extServer        string
	extPort          uint16
	localBcastTick   <-chan time.Time
	stopGlobal       chan struct{}
	globalWG         sync.WaitGroup
	forcedBcastTick  chan time.Time
	extAnnounceOK    bool
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
		go d.sendLocalAnnouncements()
	}
}

func (d *Discoverer) StartGlobal(server string, extPort uint16) {
	// Wait for any previous announcer to stop before starting a new one.
	d.globalWG.Wait()
	d.extServer = server
	d.extPort = extPort
	d.stopGlobal = make(chan struct{})
	d.globalWG.Add(1)
	go d.sendExternalAnnouncements()
}

func (d *Discoverer) StopGlobal() {
	if d.stopGlobal != nil {
		close(d.stopGlobal)
		d.globalWG.Wait()
	}
}

func (d *Discoverer) ExtAnnounceOK() bool {
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
	} else if len(d.extServer) != 0 {
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
	var pkt = Announce{
		Magic: AnnouncementMagic,
		This:  Device{d.myID[:], addrs},
	}
	return pkt.MarshalXDR()
}

func (d *Discoverer) sendLocalAnnouncements() {
	var addrs = resolveAddrs(d.listenAddrs)

	var pkt = Announce{
		Magic: AnnouncementMagic,
		This:  Device{d.myID[:], addrs},
	}
	msg := pkt.MarshalXDR()

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

func (d *Discoverer) sendExternalAnnouncements() {
	defer d.globalWG.Done()

	remote, err := net.ResolveUDPAddr("udp", d.extServer)
	for err != nil {
		l.Warnf("Global discovery: %v; trying again in %v", err, d.errorRetryIntv)
		time.Sleep(d.errorRetryIntv)
		remote, err = net.ResolveUDPAddr("udp", d.extServer)
	}

	conn, err := net.ListenUDP("udp", nil)
	for err != nil {
		l.Warnf("Global discovery: %v; trying again in %v", err, d.errorRetryIntv)
		time.Sleep(d.errorRetryIntv)
		conn, err = net.ListenUDP("udp", nil)
	}

	var buf []byte
	if d.extPort != 0 {
		var pkt = Announce{
			Magic: AnnouncementMagic,
			This:  Device{d.myID[:], []Address{{Port: d.extPort}}},
		}
		buf = pkt.MarshalXDR()
	} else {
		buf = d.announcementPkt()
	}

	var bcastTick = time.Tick(d.globalBcastIntv)
	var errTick <-chan time.Time

	sendOneAnnouncement := func() {
		var ok bool

		if debug {
			l.Debugf("discover: send announcement -> %v\n%s", remote, hex.Dump(buf))
		}

		_, err := conn.WriteTo(buf, remote)
		if err != nil {
			if debug {
				l.Debugln("discover: warning:", err)
			}
			ok = false
		} else {
			// Verify that the announce server responds positively for our device ID

			time.Sleep(1 * time.Second)
			res := d.externalLookup(d.myID)
			if debug {
				l.Debugln("discover: external lookup check:", res)
			}
			ok = len(res) > 0
		}

		d.extAnnounceOKmut.Lock()
		d.extAnnounceOK = ok
		d.extAnnounceOKmut.Unlock()

		if ok {
			errTick = nil
		} else if errTick != nil {
			errTick = time.Tick(d.errorRetryIntv)
		}
	}

	// Announce once, immediately
	sendOneAnnouncement()

loop:
	for {
		select {
		case <-d.stopGlobal:
			break loop

		case <-errTick:
			sendOneAnnouncement()

		case <-bcastTick:
			sendOneAnnouncement()
		}
	}

	if debug {
		l.Debugln("discover: stopping global")
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
	extIP, err := net.ResolveUDPAddr("udp", d.extServer)
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	conn, err := net.DialUDP("udp", nil, extIP)
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}
	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	buf := Query{QueryMagic, device[:]}.MarshalXDR()
	_, err = conn.Write(buf)
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
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
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	if debug {
		l.Debugf("discover: read external:\n%s", hex.Dump(buf[:n]))
	}

	var pkt Announce
	err = pkt.UnmarshalXDR(buf[:n])
	if err != nil && err != io.EOF {
		if debug {
			l.Debugln("discover:", err)
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
