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
	direct0 := []string{"tcp://192.0.2.44:22000", "tcp://192.0.2.42:22000", "relay://192.0.2.44:443"} // prio 0
	direct1 := []string{"tcp://192.0.2.43:22000", "tcp://192.0.2.42:22000", "relay://192.0.2.45:443"} // prio 1

	// what we expect from just direct0
	direct0Sorted := []string{"relay://192.0.2.44:443", "tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000"}

	// what we expect from direct0+direct1
	totalSorted := []string{
		// first prio 0, sorted
		"relay://192.0.2.44:443", "tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000",
		// then prio 1
		"relay://192.0.2.45:443", "tcp://192.0.2.43:22000",
		// no duplicate .42
	}

	c := NewCachingMux()
	c.ServeBackground()
	defer c.Stop()

	// Add a fake discovery service and verify we get it's answers through the
	// cache.

	f1 := &fakeDiscovery{direct0}
	c.Add(f1, time.Minute, 0, 0)

	addrs, err := c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(addrs, direct0Sorted) {
		t.Errorf("Incorrect addresses; %+v != %+v", addrs, direct0Sorted)
	}

	// Add one more that answers in the same way and check that we don't
	// duplicate or otherwise mess up the responses now.

	f2 := &fakeDiscovery{direct1}
	c.Add(f2, time.Minute, 0, 1)

	addrs, err = c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(addrs, totalSorted) {
		t.Errorf("Incorrect addresses; %+v != %+v", addrs, totalSorted)
	}
}

type fakeDiscovery struct {
	addresses []string
}

func (f *fakeDiscovery) Lookup(deviceID protocol.DeviceID) (addresses []string, err error) {
	return f.addresses, nil
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
