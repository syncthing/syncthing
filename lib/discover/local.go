// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"bytes"
	"encoding/hex"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/beacon"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture"
)

type localClient struct {
	*suture.Supervisor
	myID     protocol.DeviceID
	addrList AddressLister
	name     string

	beacon          beacon.Interface
	localBcastStart time.Time
	localBcastTick  <-chan time.Time
	forcedBcastTick chan time.Time

	*cache
}

const (
	BroadcastInterval = 30 * time.Second
	CacheLifeTime     = 3 * BroadcastInterval
)

func NewLocal(id protocol.DeviceID, addr string, addrList AddressLister) (FinderService, error) {
	c := &localClient{
		Supervisor:      suture.NewSimple("local"),
		myID:            id,
		addrList:        addrList,
		localBcastTick:  time.Tick(BroadcastInterval),
		forcedBcastTick: make(chan time.Time),
		localBcastStart: time.Now(),
		cache:           newCache(),
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if len(host) == 0 {
		// A broadcast client
		c.name = "IPv4 local"
		bcPort, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		c.startLocalIPv4Broadcasts(bcPort)
	} else {
		// A multicast client
		c.name = "IPv6 local"
		c.startLocalIPv6Multicasts(addr)
	}

	go c.sendLocalAnnouncements()

	return c, nil
}

func (c *localClient) startLocalIPv4Broadcasts(localPort int) {
	c.beacon = beacon.NewBroadcast(localPort)
	c.Add(c.beacon)
	go c.recvAnnouncements(c.beacon)
}

func (c *localClient) startLocalIPv6Multicasts(localMCAddr string) {
	c.beacon = beacon.NewMulticast(localMCAddr)
	c.Add(c.beacon)
	go c.recvAnnouncements(c.beacon)
}

// Lookup returns a list of addresses the device is available at.
func (c *localClient) Lookup(device protocol.DeviceID) (addresses []string, err error) {
	if cache, ok := c.Get(device); ok {
		if time.Since(cache.when) < CacheLifeTime {
			addresses = cache.Addresses
		}
	}

	return
}

func (c *localClient) String() string {
	return c.name
}

func (c *localClient) Error() error {
	return c.beacon.Error()
}

func (c *localClient) announcementPkt() Announce {
	var addrs []Address
	for _, addr := range c.addrList.AllAddresses() {
		addrs = append(addrs, Address{
			URL: addr,
		})
	}

	return Announce{
		Magic: AnnouncementMagic,
		This: Device{
			ID:        c.myID[:],
			Addresses: addrs,
		},
	}
}

func (c *localClient) sendLocalAnnouncements() {
	var pkt = c.announcementPkt()
	msg := pkt.MustMarshalXDR()

	for {
		c.beacon.Send(msg)

		select {
		case <-c.localBcastTick:
		case <-c.forcedBcastTick:
		}
	}
}

func (c *localClient) recvAnnouncements(b beacon.Interface) {
	for {
		buf, addr := b.Recv()

		var pkt Announce
		err := pkt.UnmarshalXDR(buf)
		if err != nil && err != io.EOF {
			l.Debugf("discover: Failed to unmarshal local announcement from %s:\n%s", addr, hex.Dump(buf))
			continue
		}

		if pkt.Magic != AnnouncementMagic {
			l.Debugf("discover: Incorrect magic from %s: %s != %s", addr, pkt.Magic, AnnouncementMagic)
			continue
		}

		l.Debugf("discover: Received local announcement from %s for %s", addr, protocol.DeviceIDFromBytes(pkt.This.ID))

		var newDevice bool
		if !bytes.Equal(pkt.This.ID, c.myID[:]) {
			newDevice = c.registerDevice(addr, pkt.This)
		}

		if newDevice {
			// Force a transmit to announce ourselves, if we are ready to do
			// so right away.
			select {
			case c.forcedBcastTick <- time.Now():
			default:
			}
		}
	}
}

func (c *localClient) registerDevice(src net.Addr, device Device) bool {
	var id protocol.DeviceID
	copy(id[:], device.ID)

	// Remember whether we already had a valid cache entry for this device.

	ce, existsAlready := c.Get(id)
	isNewDevice := !existsAlready || time.Since(ce.when) > CacheLifeTime

	// Any empty or unspecified addresses should be set to the source address
	// of the announcement. We also skip any addresses we can't parse.

	l.Debugln("discover: Registering addresses for", id)
	var validAddresses []string
	for _, addr := range device.Addresses {
		u, err := url.Parse(addr.URL)
		if err != nil {
			continue
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", u.Host)
		if err != nil {
			continue
		}

		if len(tcpAddr.IP) == 0 || tcpAddr.IP.IsUnspecified() {
			host, _, err := net.SplitHostPort(src.String())
			if err != nil {
				continue
			}
			u.Host = net.JoinHostPort(host, strconv.Itoa(tcpAddr.Port))
			l.Debugf("discover: Reconstructed URL is %#v", u)
			validAddresses = append(validAddresses, u.String())
			l.Debugf("discover: Replaced address %v in %s to get %s", tcpAddr.IP, addr.URL, u.String())
		} else {
			validAddresses = append(validAddresses, addr.URL)
			l.Debugf("discover: Accepted address %s verbatim", addr.URL)
		}
	}

	c.Set(id, CacheEntry{
		Addresses: validAddresses,
		when:      time.Now(),
		found:     true,
	})

	if isNewDevice {
		events.Default.Log(events.DeviceDiscovered, map[string]interface{}{
			"device": id.String(),
			"addrs":  validAddresses,
		})
	}

	return isNewDevice
}
