// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"reflect"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestCacheUnique(t *testing.T) {
	addresses0 := []string{"tcp://192.0.2.44:22000", "tcp://192.0.2.42:22000"} // prio 0
	addresses1 := []string{"tcp://192.0.2.43:22000", "tcp://192.0.2.42:22000"} // prio 1

	// what we expect from just addresses0
	addresses0Sorted := []string{"tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000"}

	// what we expect from addresses0+addresses1
	totalSorted := []string{
		// first prio 0, sorted
		"tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000",
		// then prio 1
		"tcp://192.0.2.43:22000",
		// no duplicate .42
	}

	c := NewCachingMux()
	c.(*cachingMux).ServeBackground()
	defer c.Stop()

	// Add a fake discovery service and verify we get it's answers through the
	// cache.

	f1 := &fakeDiscovery{addresses0}
	c.Add(f1, time.Minute, 0, 0, 0)

	addr, err := c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(addr, addresses0Sorted) {
		t.Errorf("Incorrect addresses; %+v != %+v", addr, addresses0Sorted)
	}

	// Add one more that answers in the same way and check that we don't
	// duplicate or otherwise mess up the responses now.

	f2 := &fakeDiscovery{addresses1}
	c.Add(f2, time.Minute, 0, 1, 0)

	addr, err = c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(addr, totalSorted) {
		t.Errorf("Incorrect addresses; %+v != %+v", addr, totalSorted)
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

func TestCacheSlowLookup(t *testing.T) {
	c := NewCachingMux()
	c.(*cachingMux).ServeBackground()
	defer c.Stop()

	// Add a slow discovery service.

	started := make(chan struct{})
	f1 := &slowDiscovery{time.Second, started}
	c.Add(f1, time.Minute, 0, 0, 0)

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

func (f *slowDiscovery) Lookup(deviceID protocol.DeviceID) (addresses []string, err error) {
	close(f.started)
	time.Sleep(f.delay)
	return nil, nil
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

func TestCacheDelay(t *testing.T) {
	c := NewCachingMux()
	c.(*cachingMux).ServeBackground()
	defer c.Stop()

	// Add a source that is not used yet and has low priority

	const first = "tcp://first:22000"
	c.Add(&fakeDiscovery{[]string{first}}, time.Minute, 0, 20, 100*time.Millisecond)

	// Add a source that is used immediately and has high priority

	const second = "tcp://second:22000"
	c.Add(&fakeDiscovery{[]string{second}}, time.Minute, 0, 10, 0)

	// Verify that we only get an answer from the second source

	res, err := c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("Incorrect result length %d should be 1", len(res))
	}
	if res[0] != second {
		t.Errorf("Incorrect response %q should be %q", res[0], second)
	}

	// Wait for the first source to become valid

	time.Sleep(150 * time.Millisecond)

	// We should now get back both answers, in the right order

	res, err = c.Lookup(protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("Incorrect result length %d should be 2", len(res))
	}
	if res[0] != second {
		t.Errorf("Incorrect response %q should be %q", res[0], second)
	}
	if res[1] != first {
		t.Errorf("Incorrect response %q should be %q", res[1], first)
	}
}
