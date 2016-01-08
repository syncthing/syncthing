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
	direct0 := []string{"tcp://192.0.2.44:22000", "tcp://192.0.2.42:22000"} // prio 0
	direct1 := []string{"tcp://192.0.2.43:22000", "tcp://192.0.2.42:22000"} // prio 1

	// what we expect from just direct0
	direct0Sorted := []string{"tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000"}

	// what we expect from direct0+direct1
	totalSorted := []string{
		// first prio 0, sorted
		"tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000",
		// then prio 1
		"tcp://192.0.2.43:22000",
		// no duplicate .42
	}

	relays := []Relay{{URL: "relay://192.0.2.44:443"}, {URL: "tcp://192.0.2.45:443"}}

	c := NewCachingMux()
	c.ServeBackground()
	defer c.Stop()

	// Add a fake discovery service and verify we get it's answers through the
	// cache.

	f1 := &fakeDiscovery{direct0, relays}
	c.Add(f1, time.Minute, 0, 0)

	dir, rel, err := c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dir, direct0Sorted) {
		t.Errorf("Incorrect direct; %+v != %+v", dir, direct0Sorted)
	}
	if !reflect.DeepEqual(rel, relays) {
		t.Errorf("Incorrect relays; %+v != %+v", rel, relays)
	}

	// Add one more that answers in the same way and check that we don't
	// duplicate or otherwise mess up the responses now.

	f2 := &fakeDiscovery{direct1, relays}
	c.Add(f2, time.Minute, 0, 1)

	dir, rel, err = c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dir, totalSorted) {
		t.Errorf("Incorrect direct; %+v != %+v", dir, totalSorted)
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

func TestCacheSlowLookup(t *testing.T) {
	c := NewCachingMux()
	c.ServeBackground()
	defer c.Stop()

	// Add a slow discovery service.

	started := make(chan struct{})
	f1 := &slowDiscovery{time.Second, started}
	c.Add(f1, time.Minute, 0, 0)

	// Start a lookup, which will take at least a second

	t0 := time.Now()
	go c.Lookup(protocol.LocalDeviceID)
	<-started // The slow lookup method has been called so we're inside the lock

	// It should be possible to get ChildErrors while it's running

	c.ChildErrors()

	// Only a small amount of time should have passed, not the full second

	diff := time.Since(t0)
	if diff > 500*time.Millisecond {
		t.Error("ChildErrors was blocked for", diff)
	}
}

type slowDiscovery struct {
	delay   time.Duration
	started chan struct{}
}

func (f *slowDiscovery) Lookup(deviceID protocol.DeviceID) (direct []string, relays []Relay, err error) {
	close(f.started)
	time.Sleep(f.delay)
	return nil, nil, nil
}

func (f *slowDiscovery) Error() error {
	return nil
}

func (f *slowDiscovery) String() string {
	return "fake"
}

func (f *slowDiscovery) Cache() map[protocol.DeviceID]CacheEntry {
	return nil
}
