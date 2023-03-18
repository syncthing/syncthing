// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"context"
	"crypto/tls"
	"reflect"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

func setupCache() *manager {
	cfg := config.New(protocol.LocalDeviceID)
	cfg.Options.LocalAnnEnabled = false
	cfg.Options.GlobalAnnEnabled = false

	return NewManager(protocol.LocalDeviceID, config.Wrap("", cfg, protocol.LocalDeviceID, events.NoopLogger), tls.Certificate{}, events.NoopLogger, nil, registry.New()).(*manager)
}

func TestCacheUnique(t *testing.T) {
	addresses0 := []string{"tcp://192.0.2.44:22000", "tcp://192.0.2.42:22000"}
	addresses1 := []string{"tcp://192.0.2.43:22000", "tcp://192.0.2.42:22000"}

	// what we expect from just addresses0
	addresses0Sorted := []string{"tcp://192.0.2.42:22000", "tcp://192.0.2.44:22000"}

	// what we expect from addresses0+addresses1
	totalSorted := []string{
		"tcp://192.0.2.42:22000",
		// no duplicate .42
		"tcp://192.0.2.43:22000",
		"tcp://192.0.2.44:22000",
	}

	c := setupCache()

	// Add a fake discovery service and verify we get its answers through the
	// cache.

	f1 := &fakeDiscovery{addresses0}
	c.addLocked("f1", f1, time.Minute, 0)

	ctx := context.Background()

	addr, err := c.Lookup(ctx, protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(addr, addresses0Sorted) {
		t.Errorf("Incorrect addresses; %+v != %+v", addr, addresses0Sorted)
	}

	// Add one more that answers in the same way and check that we don't
	// duplicate or otherwise mess up the responses now.

	f2 := &fakeDiscovery{addresses1}
	c.addLocked("f2", f2, time.Minute, 0)

	addr, err = c.Lookup(ctx, protocol.LocalDeviceID)
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

func (f *fakeDiscovery) Lookup(_ context.Context, _ protocol.DeviceID) (addresses []string, err error) {
	return f.addresses, nil
}

func (*fakeDiscovery) Error() error {
	return nil
}

func (*fakeDiscovery) String() string {
	return "fake"
}

func (*fakeDiscovery) Cache() map[protocol.DeviceID]CacheEntry {
	return nil
}

func TestCacheSlowLookup(t *testing.T) {
	c := setupCache()

	// Add a slow discovery service.

	started := make(chan struct{})
	f1 := &slowDiscovery{time.Second, started}
	c.addLocked("f1", f1, time.Minute, 0)

	// Start a lookup, which will take at least a second

	t0 := time.Now()
	go c.Lookup(context.Background(), protocol.LocalDeviceID)
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

func (f *slowDiscovery) Lookup(_ context.Context, _ protocol.DeviceID) (addresses []string, err error) {
	close(f.started)
	time.Sleep(f.delay)
	return nil, nil
}

func (*slowDiscovery) Error() error {
	return nil
}

func (*slowDiscovery) String() string {
	return "fake"
}

func (*slowDiscovery) Cache() map[protocol.DeviceID]CacheEntry {
	return nil
}
