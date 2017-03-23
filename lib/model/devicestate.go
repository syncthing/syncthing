// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// The deviceState represents the the current state and activity level of a
// given device. Please enjoy the following state diagram that illustrates
// the possible states and their transitions:
//
//              +---------------+
//              |               |
//              | Disconnected  |
//              |               |
//              +---------------+
//                 ^         |
//                 |    Connected()
//                 |         |
//          Disconnected()   |
//                 |         v
// +--------------------------------------+
// |             +-------------+          |--PreparingIndex()---+
// |             |             |          |                     v
// |          +->|    Idle     |--+       |              +-------------+
// |          |  |             |  |       |              |  Preparing  |
// |          |  +-------------+  |       |              |    Index    |
// |          |                   |       |              +-------------+
// |          |            SyncActivity() |                     |
// |          |                   |       |                     | SendingIndex()
// | deviceIdleThreshold          |       |                     v
// |          |                   |       |              +-------------+
// |          |  +-------------+  |       |              |   Sending   |
// |          |  |             |  |       |              |    Index    |
// |          +--|   Syncing   |<-+       |              +-------------+
// |             |             |          |                     |
// |             +-------------+          |<-DoneSendingIndex()-+
// +--------------------------------------+
//
// The "Connected" and "Disconnected" edges emit DeviceConnected /
// DeviceDisconnected events, for legacy reasons. The other edges emit
// DeviceStateChanged events with the event data containing the old and new
// state for the device.
//
// Given that the state is for an entire device and that it may have many
// folders in different states, the actual precedence order to determine the
// device state is:
//
// - Syncing, if any folder is doing that
// - PreparingIndex, if any folder is doing that
// - SendingIndex, if any folder is doing that
// - Idle, if connected
// - Disconnected
//
// This means that while PreparingIndex() and SendingIndex() can be called
// while the device is otherwise either Idle or Syncing, those edges won't
// be taken until the device becomes Idle.
//
type deviceState int

const (
	deviceStateDisconnected   deviceState = iota
	deviceStateSyncing                    // Have received a request within the last deviceIdleThreshold
	deviceStateIdle                       // Connected, but no recent request
	deviceStatePreparingIndex             // We are sorting index for transmission
	deviceStateSendingIndex               // We are sending index data
)

// When we have not received a Request for this long the device is
// considered idle. "var" because we want to be able to override it from
// tests.
var deviceIdleThreshold = 30 * time.Second

func (s deviceState) String() string {
	switch s {
	case deviceStateDisconnected:
		return "disconnected"
	case deviceStateSyncing:
		return "syncing"
	case deviceStateIdle:
		return "idle"
	case deviceStatePreparingIndex:
		return "preparingIndex"
	case deviceStateSendingIndex:
		return "sendingIndex"
	default:
		return "unknown"
	}
}

// deviceStateTracker keeps track of events and the current state for a
// given device
type deviceStateTracker struct {
	id protocol.DeviceID

	mut                  sync.Mutex
	connected            bool
	preparingIndex       int
	sendingIndex         int
	lastActivity         time.Time
	prevState            deviceState
	activityTimeoutTimer *time.Timer
}

func newDeviceStateTracker(id protocol.DeviceID) *deviceStateTracker {
	s := &deviceStateTracker{
		id:  id,
		mut: sync.NewMutex(),
	}
	s.activityTimeoutTimer = time.AfterFunc(time.Hour, s.activityTimeout)
	return s
}

// State returns the current device state.
func (s *deviceStateTracker) State() deviceState {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.stateLocked()
}

// Connected is called to indicate that we have accepted a new connection to
// the device.
func (s *deviceStateTracker) Connected(hello protocol.HelloResult, connType string, addr net.Addr) {
	s.mut.Lock()
	s.connected = true
	s.prevState = deviceStateIdle
	s.mut.Unlock()

	event := map[string]string{
		"id":            s.id.String(),
		"deviceName":    hello.DeviceName,
		"clientName":    hello.ClientName,
		"clientVersion": hello.ClientVersion,
		"type":          connType,
	}
	if addr != nil {
		event["addr"] = addr.String()
	}

	events.Default.Log(events.DeviceConnected, event)
}

// Disconnected is called to indicate that we no longer have an active
// connection to the device.
func (s *deviceStateTracker) Disconnected(err error) {
	s.mut.Lock()
	s.connected = false
	s.prevState = deviceStateDisconnected
	s.activityTimeoutTimer.Stop()
	s.mut.Unlock()

	events.Default.Log(events.DeviceDisconnected, map[string]string{
		"id":    s.id.String(),
		"error": err.Error(),
	})
}

// SyncActivity is called to indicate that he device is downloading data
// from us.
func (s *deviceStateTracker) SyncActivity() {
	s.mut.Lock()
	s.lastActivity = time.Now()
	s.activityTimeoutTimer.Reset(deviceIdleThreshold)
	s.stateChangedLocked()
	s.mut.Unlock()
}

// PreparingIndex is called to indicate that we are preparing index data to
// send to the device.
func (s *deviceStateTracker) PreparingIndex() {
	s.mut.Lock()
	s.preparingIndex++
	s.stateChangedLocked()
	s.mut.Unlock()
}

// SendingIndex is called to indicate that we have started sending index
// data to the device.
func (s *deviceStateTracker) SendingIndex() {
	s.mut.Lock()
	s.preparingIndex--
	if s.preparingIndex < 0 {
		panic("unmatched call to SendingIndex (compared to PreparingIndex)")
	}
	s.sendingIndex++
	s.stateChangedLocked()
	s.mut.Unlock()
}

// DoneSendingIndex is called to indicate that we are no longer sending
// index data.
func (s *deviceStateTracker) DoneSendingIndex() {
	s.mut.Lock()
	s.sendingIndex--
	if s.sendingIndex < 0 {
		panic("unmatched call to DoneSendingIndex (compared to SendingIndex)")
	}
	s.stateChangedLocked()
	s.mut.Unlock()
}

// stateLocked calculates the current state based on activity reports and
// timing. It must be called with the mutex held.
func (s *deviceStateTracker) stateLocked() deviceState {
	switch {
	case time.Since(s.lastActivity) < deviceIdleThreshold:
		return deviceStateSyncing
	case s.preparingIndex > 0:
		return deviceStatePreparingIndex
	case s.sendingIndex > 0:
		return deviceStateSendingIndex
	case s.connected:
		return deviceStateIdle
	default:
		return deviceStateDisconnected
	}
}

// stateChangedLocked emits the appropriate event if the state has changed
// since last time it was called. It must be called with the mutex held.
func (s *deviceStateTracker) stateChangedLocked() {
	state := s.stateLocked()
	if state != s.prevState {
		events.Default.Log(events.DeviceStateChanged, map[string]string{
			"id":   s.id.String(),
			"from": s.prevState.String(),
			"to":   state.String(),
		})
		s.prevState = state
	}
}

// activityTimeout is called when the timeout expires after a previous SyncActivity call.
func (s *deviceStateTracker) activityTimeout() {
	s.mut.Lock()
	// We simply call stateChangedLocked who will notice that it's long
	// enough since the last SyncActivity that we are now Idle and emit the
	// appropriate event.
	s.stateChangedLocked()
	s.mut.Unlock()
}

// deviceStateMap is a concurrency safe container for deviceStateTrackers
type deviceStateMap struct {
	mut    sync.Mutex
	states map[protocol.DeviceID]*deviceStateTracker
}

func newDeviceStateMap() *deviceStateMap {
	return &deviceStateMap{
		mut:    sync.NewMutex(),
		states: make(map[protocol.DeviceID]*deviceStateTracker),
	}
}

func (s *deviceStateMap) Get(id protocol.DeviceID) *deviceStateTracker {
	s.mut.Lock()
	st, ok := s.states[id]
	if !ok {
		st = newDeviceStateTracker(id)
		s.states[id] = st
	}
	s.mut.Unlock()
	return st
}

func (s *deviceStateMap) Delete(id protocol.DeviceID) {
	s.mut.Lock()
	delete(s.states, id)
	s.mut.Unlock()
}
