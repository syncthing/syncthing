// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"net/url"
	"time"

	"testing"

	"github.com/syncthing/protocol"
)

type DummyClient struct {
	url          *url.URL
	lookups      []protocol.DeviceID
	lookupRet    Announce
	stops        int
	statusRet    bool
	statusChecks int
}

func (c *DummyClient) Lookup(device protocol.DeviceID) (Announce, error) {
	c.lookups = append(c.lookups, device)
	return c.lookupRet, nil
}

func (c *DummyClient) StatusOK() bool {
	c.statusChecks++
	return c.statusRet
}

func (c *DummyClient) Stop() {
	c.stops++
}

func (c *DummyClient) Address() string {
	return c.url.String()
}

func TestGlobalDiscovery(t *testing.T) {
	c1 := &DummyClient{
		statusRet: false,
		lookupRet: Announce{
			Magic: AnnouncementMagic,
			This: Device{
				ID:        protocol.LocalDeviceID[:],
				Addresses: []string{"test.com:1234"},
				Relays:    nil,
			},
			Extra: nil,
		},
	}

	c2 := &DummyClient{
		statusRet: true,
		lookupRet: Announce{
			Magic: AnnouncementMagic,
			This: Device{
				ID:        protocol.LocalDeviceID[:],
				Addresses: nil,
				Relays:    nil,
			},
			Extra: nil,
		},
	}

	c3 := &DummyClient{
		statusRet: true,
		lookupRet: Announce{
			Magic: AnnouncementMagic,
			This: Device{
				ID:        protocol.LocalDeviceID[:],
				Addresses: []string{"best.com:2345"},
				Relays:    nil,
			},
			Extra: nil,
		},
	}

	clients := []*DummyClient{c1, c2}

	Register("test1", func(uri *url.URL, ann Announcer) (Client, error) {
		c := clients[0]
		clients = clients[1:]
		c.url = uri
		return c, nil
	})

	Register("test2", func(uri *url.URL, ann Announcer) (Client, error) {
		c3.url = uri
		return c3, nil
	})

	d := NewDiscoverer(device, []string{}, nil)
	d.localBcastStart = time.Time{}
	servers := []string{
		"test1://123.123.123.123:1234",
		"test1://23.23.23.23:234",
		"test2://234.234.234.234.2345",
	}
	d.StartGlobal(servers, 1234)

	if len(d.clients) != 3 {
		t.Fatal("Wrong number of clients")
	}

	status := d.ExtAnnounceOK()

	for _, c := range []*DummyClient{c1, c2, c3} {
		if status[c.url.String()] != c.statusRet || c.statusChecks != 1 {
			t.Fatal("Wrong status")
		}
	}

	addrs, _ := d.Lookup(device)
	if len(addrs) != 2 {
		t.Fatal("Wrong number of addresses", addrs)
	}

	for _, addr := range []string{"test.com:1234", "best.com:2345"} {
		found := false
		for _, laddr := range addrs {
			if laddr == addr {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Couldn't find", addr)
		}
	}

	for _, c := range []*DummyClient{c1, c2, c3} {
		if len(c.lookups) != 1 || c.lookups[0] != device {
			t.Fatal("Wrong lookups")
		}
	}

	addrs, _ = d.Lookup(device)
	if len(addrs) != 2 {
		t.Fatal("Wrong number of addresses", addrs)
	}

	// Answer should be cached, so number of lookups should have not increased
	for _, c := range []*DummyClient{c1, c2, c3} {
		if len(c.lookups) != 1 || c.lookups[0] != device {
			t.Fatal("Wrong lookups")
		}
	}

	d.StopGlobal()

	for _, c := range []*DummyClient{c1, c2, c3} {
		if c.stops != 1 {
			t.Fatal("Wrong number of stops")
		}
	}
}
