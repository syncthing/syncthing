// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/thejerf/suture/v4"
	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/beacon"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/svcutil"
)

type localClient struct {
	*suture.Supervisor
	myID     protocol.DeviceID
	addrList AddressLister
	name     string
	evLogger events.Logger

	beacon          beacon.Interface
	localBcastStart time.Time
	localBcastTick  <-chan time.Time
	forcedBcastTick chan time.Time

	*cache
}

const (
	BroadcastInterval = 30 * time.Second
	CacheLifeTime     = 3 * BroadcastInterval
	Magic             = uint32(0x2EA7D90B) // same as in BEP
	v13Magic          = uint32(0x7D79BC40) // previous version
)

func NewLocal(id protocol.DeviceID, addr string, addrList AddressLister, evLogger events.Logger) (FinderService, error) {
	c := &localClient{
		Supervisor:      suture.New("local", svcutil.SpecWithDebugLogger(l)),
		myID:            id,
		addrList:        addrList,
		evLogger:        evLogger,
		localBcastTick:  time.NewTicker(BroadcastInterval).C,
		forcedBcastTick: make(chan time.Time),
		localBcastStart: time.Now(),
		cache:           newCache(),
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if host == "" {
		// A broadcast client
		c.name = "IPv4 local"
		bcPort, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		c.beacon = beacon.NewBroadcast(bcPort)
	} else {
		// A multicast client
		c.name = "IPv6 local"
		c.beacon = beacon.NewMulticast(addr)
	}
	c.Add(c.beacon)
	c.Add(svcutil.AsService(c.recvAnnouncements, fmt.Sprintf("%s/recv", c)))

	c.Add(svcutil.AsService(c.sendLocalAnnouncements, fmt.Sprintf("%s/sendLocal", c)))

	return c, nil
}

// Lookup returns a list of addresses the device is available at.
func (c *localClient) Lookup(_ context.Context, device protocol.DeviceID) (addresses []string, err error) {
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

// announcementPkt appends the local discovery packet to send to msg. Returns
// true if the packet should be sent, false if there is nothing useful to
// send.
func (c *localClient) announcementPkt(instanceID int64, msg []byte) ([]byte, bool) {
	addrs := c.addrList.AllAddresses()

	// remove all addresses which are not dialable
	addrs = filterUndialableLocal(addrs)

	// do not leak relay tokens to discovery
	addrs = sanitizeRelayAddresses(addrs)

	if len(addrs) == 0 {
		// Nothing to announce
		return msg, false
	}

	pkt := &discoproto.Announce{
		Id:         c.myID[:],
		Addresses:  addrs,
		InstanceId: instanceID,
	}
	bs, _ := proto.Marshal(pkt)

	if pktLen := 4 + len(bs); cap(msg) < pktLen {
		msg = make([]byte, 0, pktLen)
	}
	msg = msg[:4]
	binary.BigEndian.PutUint32(msg, Magic)
	msg = append(msg, bs...)

	return msg, true
}

func (c *localClient) sendLocalAnnouncements(ctx context.Context) error {
	var msg []byte
	var ok bool
	instanceID := rand.Int63()
	for {
		if msg, ok = c.announcementPkt(instanceID, msg[:0]); ok {
			c.beacon.Send(msg)
		}

		select {
		case <-c.localBcastTick:
		case <-c.forcedBcastTick:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *localClient) recvAnnouncements(ctx context.Context) error {
	b := c.beacon
	warnedAbout := make(map[string]bool)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		buf, addr := b.Recv()
		if addr == nil {
			continue
		}
		if len(buf) < 4 {
			l.Debugf("discover: short packet from %s", addr.String())
			continue
		}

		magic := binary.BigEndian.Uint32(buf)
		switch magic {
		case Magic:
			// All good

		case v13Magic:
			// Old version
			if !warnedAbout[addr.String()] {
				l.Warnf("Incompatible (v0.13) local discovery packet from %v - upgrade that device to connect", addr)
				warnedAbout[addr.String()] = true
			}
			continue

		default:
			l.Debugf("discover: Incorrect magic %x from %s", magic, addr)
			continue
		}

		var pkt discoproto.Announce
		err := proto.Unmarshal(buf[4:], &pkt)
		if err != nil && !errors.Is(err, io.EOF) {
			l.Debugf("discover: Failed to unmarshal local announcement from %s (%s):\n%s", addr, err, hex.Dump(buf[4:]))
			continue
		}

		id, _ := protocol.DeviceIDFromBytes(pkt.Id)
		l.Debugf("discover: Received local announcement from %s for %s", addr, id)

		var newDevice bool
		if !bytes.Equal(pkt.Id, c.myID[:]) {
			newDevice = c.registerDevice(addr, &pkt)
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

func (c *localClient) registerDevice(src net.Addr, device *discoproto.Announce) bool {
	// Remember whether we already had a valid cache entry for this device.
	// If the instance ID has changed the remote device has restarted since
	// we last heard from it, so we should treat it as a new device.

	id, err := protocol.DeviceIDFromBytes(device.Id)
	if err != nil {
		l.Debugf("discover: Failed to parse device ID %x: %v", device.Id, err)
		return false
	}

	ce, existsAlready := c.Get(id)
	isNewDevice := !existsAlready || time.Since(ce.when) > CacheLifeTime || ce.instanceID != device.InstanceId

	// Any empty or unspecified addresses should be set to the source address
	// of the announcement. We also skip any addresses we can't parse.

	l.Debugln("discover: Registering addresses for", id)
	var validAddresses []string
	for _, addr := range device.Addresses {
		u, err := url.Parse(addr)
		if err != nil {
			continue
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", u.Host)
		if err != nil {
			continue
		}

		if len(tcpAddr.IP) == 0 || tcpAddr.IP.IsUnspecified() {
			srcAddr, err := net.ResolveTCPAddr("tcp", src.String())
			if err != nil {
				continue
			}

			// Do not use IPv6 source address if requested scheme is tcp4
			if u.Scheme == "tcp4" && srcAddr.IP.To4() == nil {
				continue
			}

			// Do not use IPv4 source address if requested scheme is tcp6
			if u.Scheme == "tcp6" && srcAddr.IP.To4() != nil {
				continue
			}

			host, _, err := net.SplitHostPort(src.String())
			if err != nil {
				continue
			}
			u.Host = net.JoinHostPort(host, strconv.Itoa(tcpAddr.Port))
			l.Debugf("discover: Reconstructed URL is %v", u)
			validAddresses = append(validAddresses, u.String())
			l.Debugf("discover: Replaced address %v in %s to get %s", tcpAddr.IP, addr, u.String())
		} else {
			validAddresses = append(validAddresses, addr)
			l.Debugf("discover: Accepted address %s verbatim", addr)
		}
	}

	c.Set(id, CacheEntry{
		Addresses:  validAddresses,
		when:       time.Now(),
		found:      true,
		instanceID: device.InstanceId,
	})

	if isNewDevice {
		c.evLogger.Log(events.DeviceDiscovered, map[string]interface{}{
			"device": id.String(),
			"addrs":  validAddresses,
		})
	}

	return isNewDevice
}

// filterUndialableLocal returns the list of addresses after removing any
// localhost, multicast, broadcast or port-zero addresses.
func filterUndialableLocal(addrs []string) []string {
	filtered := addrs[:0]
	for _, addr := range addrs {
		u, err := url.Parse(addr)
		if err != nil {
			continue
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", u.Host)
		if err != nil {
			continue
		}

		switch {
		case len(tcpAddr.IP) == 0:
		case tcpAddr.Port == 0:
		case tcpAddr.IP.IsGlobalUnicast(), tcpAddr.IP.IsLinkLocalUnicast(), tcpAddr.IP.IsUnspecified():
			filtered = append(filtered, addr)
		}
	}
	return filtered
}

func sanitizeRelayAddresses(addrs []string) []string {
	filtered := addrs[:0]
	allowlist := []string{"id"}

	for _, addr := range addrs {
		u, err := url.Parse(addr)
		if err != nil {
			continue
		}

		if u.Scheme == "relay" {
			s := url.Values{}
			q := u.Query()

			for _, w := range allowlist {
				if q.Has(w) {
					s.Add(w, q.Get(w))
				}
			}

			u.RawQuery = s.Encode()
			addr = u.String()
		}

		filtered = append(filtered, addr)
	}
	return filtered
}
