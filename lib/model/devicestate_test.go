// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"net"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDeviceStateEvents(t *testing.T) {
	// Set the device idle threshold to 1 ms to we can test quickly, and
	// restore it when the test ends.

	oldThres := deviceIdleThreshold
	defer func() { deviceIdleThreshold = oldThres }()
	deviceIdleThreshold = time.Millisecond

	// Subscribe to events and create a device state map.

	sub := events.NewBufferedSubscription(events.Default.Subscribe(events.AllEvents), 1)
	dss := newDeviceStateMap()
	st := dss.Get(protocol.LocalDeviceID)

	// Helper for event checking

	lastEventID := 0
	expectEvent := func(comment string, ev events.EventType) {
		evs := sub.Since(lastEventID, nil, time.Second)
		if len(evs) != 1 {
			t.Fatalf("%s: should have got exactly one event, not %d", comment, len(evs))
		}
		if evs[0].Type != ev {
			t.Fatalf("%s: event should have been %v, not %v", comment, ev, evs[0].Type)
		}
		lastEventID = evs[0].SubscriptionID
	}

	expectState := func(comment string, es deviceState) {
		if state := st.State(); state != es {
			t.Fatalf("%s: device state should be %v, not %v", comment, es, state)
		}
	}

	// Device is disconnected by default

	expectState("default", deviceStateDisconnected)

	// Connect

	st.Connected(protocol.HelloResult{}, "fake", &net.IPAddr{})
	expectEvent("connect", events.DeviceConnected)
	expectState("connect", deviceStateIdle)

	// Preparing index

	st.PreparingIndex()
	expectEvent("preparing index", events.DeviceStateChanged)
	expectState("preparing index", deviceStatePreparingIndex)

	// Sync (takes precedence over the index stuff)

	st.SyncActivity()
	expectEvent("sync 1", events.DeviceStateChanged)
	expectState("sync 1", deviceStateSyncing)

	// Device should now become idle. By waiting for the event we wait for
	// the deviceIdleThreshold to kick in. The state changes back to
	// preparing index.

	expectEvent("idle", events.DeviceStateChanged)
	expectState("idle", deviceStatePreparingIndex)

	// Sending index

	st.SendingIndex()
	expectEvent("sending index", events.DeviceStateChanged)
	expectState("sending index", deviceStateSendingIndex)

	// Done sending indexes. Device should now become idle again.

	st.DoneSendingIndex()
	expectEvent("done sending index", events.DeviceStateChanged)
	expectState("done sending index", deviceStateIdle)

	// Sync

	st.SyncActivity()
	expectEvent("sync 2", events.DeviceStateChanged)
	expectState("sync 2", deviceStateSyncing)

	// Device should now become idle.

	expectEvent("idle", events.DeviceStateChanged)
	expectState("idle", deviceStateIdle)
}
