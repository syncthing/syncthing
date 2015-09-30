// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"reflect"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestCacheUnique(t *testing.T) {
	// Direct addresses should be sorted by prio
	direct := []string{"tcp://192.0.2.43:22000?prio=8", "tcp://192.0.2.44:22000?prio=10", "tcp://192.0.2.42:22000?prio=20"}
	relays := []Relay{{URL: "relay://192.0.2.44:443"}, {URL: "tcp://192.0.2.45:443"}}

	c := NewCachingMux()
	c.ServeBackground()
	defer c.Stop()

	// Add a fake discovery service and verify we get it's answers through the
	// cache.

	f1 := &fakeDiscovery{direct, relays}
	c.Add(f1, time.Minute, 0)

	dir, rel, err := c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dir, direct) {
		t.Errorf("Incorrect direct; %+v != %+v", dir, direct)
	}
	if !reflect.DeepEqual(rel, relays) {
		t.Errorf("Incorrect relays; %+v != %+v", rel, relays)
	}

	// Add one more that answers in the same way and check that we don't
	// duplicate or otherwise mess up the responses now.

	f2 := &fakeDiscovery{direct, relays}
	c.Add(f2, time.Minute, 0)

	dir, rel, err = c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dir, direct) {
		t.Errorf("Incorrect direct; %+v != %+v", dir, direct)
	}
	if !reflect.DeepEqual(rel, relays) {
		t.Errorf("Incorrect relays; %+v != %+v", rel, relays)
	}
}

type fakeDiscovery struct {
	direct []string
	relays []Relay
}

func (f *fakeDiscovery) Lookup(deviceID protocol.DeviceID) (direct []string, relays []Relay, err error) {
	return f.direct, f.relays, nil
}

func (f *fakeDiscovery) Error() error {
	return nil
}

func (f *fakeDiscovery) String() string {
	return "fake"
}

func (f *fakeDiscovery) Cache() map[protocol.DeviceID]CacheEntry {
	return nil
}
