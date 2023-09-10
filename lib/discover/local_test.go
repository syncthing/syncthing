// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestLocalInstanceID(t *testing.T) {
	c, err := NewLocal(protocol.LocalDeviceID, ":0", &fakeAddressLister{}, events.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go c.Serve(ctx)
	defer cancel()

	lc := c.(*localClient)

	p0, ok := lc.announcementPkt(1, nil)
	if !ok {
		t.Fatal("unexpectedly not ok")
	}
	p1, ok := lc.announcementPkt(2, nil)
	if !ok {
		t.Fatal("unexpectedly not ok")
	}
	if bytes.Equal(p0, p1) {
		t.Error("each generated packet should have a new instance id")
	}
}

func TestLocalInstanceIDShouldTriggerNew(t *testing.T) {
	c, err := NewLocal(protocol.LocalDeviceID, ":0", &fakeAddressLister{}, events.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}

	lc := c.(*localClient)
	src := &net.UDPAddr{IP: []byte{10, 20, 30, 40}, Port: 50}

	new := lc.registerDevice(src, Announce{
		ID:         protocol.DeviceID{10, 20, 30, 40, 50, 60, 70, 80, 90},
		Addresses:  []string{"tcp://0.0.0.0:22000"},
		InstanceID: 1234567890,
	})

	if !new {
		t.Fatal("first register should be new")
	}

	new = lc.registerDevice(src, Announce{
		ID:         protocol.DeviceID{10, 20, 30, 40, 50, 60, 70, 80, 90},
		Addresses:  []string{"tcp://0.0.0.0:22000"},
		InstanceID: 1234567890,
	})

	if new {
		t.Fatal("second register should not be new")
	}

	new = lc.registerDevice(src, Announce{
		ID:         protocol.DeviceID{42, 10, 20, 30, 40, 50, 60, 70, 80, 90},
		Addresses:  []string{"tcp://0.0.0.0:22000"},
		InstanceID: 1234567890,
	})

	if !new {
		t.Fatal("new device ID should be new")
	}

	new = lc.registerDevice(src, Announce{
		ID:         protocol.DeviceID{10, 20, 30, 40, 50, 60, 70, 80, 90},
		Addresses:  []string{"tcp://0.0.0.0:22000"},
		InstanceID: 91234567890,
	})

	if !new {
		t.Fatal("new instance ID should be new")
	}
}

func TestFilterUndialable(t *testing.T) {
	addrs := []string{
		"quic://[2001:db8::1]:22000",             // OK
		"tcp://192.0.2.42:22000",                 // OK
		"quic://[2001:db8::1]:0",                 // remove, port zero
		"tcp://192.0.2.42:0",                     // remove, port zero
		"quic://[::]:22000",                      // OK
		"tcp://0.0.0.0:22000",                    // OK
		"tcp://[2001:db8::1]",                    // remove, no port
		"tcp://192.0.2.42",                       // remove, no port
		"tcp://foo:bar",                          // remove, host/port does not resolve
		"tcp://127.0.0.1:22000",                  // remove, not usable from outside
		"tcp://[::1]:22000",                      // remove, not usable from outside
		"tcp://224.1.2.3:22000",                  // remove, not usable from outside (multicast)
		"tcp://[fe80::9ef:dff1:b332:5e56]:55681", // OK
		"pure garbage",                           // remove, garbage
		"",                                       // remove, garbage
	}
	exp := []string{
		"quic://[2001:db8::1]:22000",
		"tcp://192.0.2.42:22000",
		"quic://[::]:22000",
		"tcp://0.0.0.0:22000",
		"tcp://[fe80::9ef:dff1:b332:5e56]:55681",
	}
	res := filterUndialableLocal(addrs)
	if fmt.Sprint(res) != fmt.Sprint(exp) {
		t.Log(res)
		t.Error("filterUndialableLocal returned invalid addresses")
	}
}
