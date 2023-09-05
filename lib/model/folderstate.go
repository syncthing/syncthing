// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/sync"
)

type folderState int

const (
	FolderIdle folderState = iota
	FolderScanning
	FolderScanWaiting
	FolderSyncWaiting
	FolderSyncPreparing
	FolderSyncing
	FolderCleaning
	FolderCleanWaiting
	FolderError
)

func (s folderState) String() string {
	switch s {
	case FolderIdle:
		return "idle"
	case FolderScanning:
		return "scanning"
	case FolderScanWaiting:
		return "scan-waiting"
	case FolderSyncWaiting:
		return "sync-waiting"
	case FolderSyncPreparing:
		return "sync-preparing"
	case FolderSyncing:
		return "syncing"
	case FolderCleaning:
		return "cleaning"
	case FolderCleanWaiting:
		return "clean-waiting"
	case FolderError:
		return "error"
	default:
		return "unknown"
	}
}

type remoteFolderState int

const (
	remoteFolderUnknown remoteFolderState = iota
	remoteFolderNotSharing
	remoteFolderPaused
	remoteFolderValid
)

func (s remoteFolderState) String() string {
	switch s {
	case remoteFolderUnknown:
		return "unknown"
	case remoteFolderNotSharing:
		return "notSharing"
	case remoteFolderPaused:
		return "paused"
	case remoteFolderValid:
		return "valid"
	default:
		return "unknown"
	}
}

func (s remoteFolderState) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

type stateTracker struct {
	folderID string
	evLogger events.Logger

	mut     sync.Mutex
	current folderState
	err     error
	changed time.Time
}

func newStateTracker(id string, evLogger events.Logger) stateTracker {
	return stateTracker{
		folderID: id,
		evLogger: evLogger,
		mut:      sync.NewMutex(),
	}
}

// setState sets the new folder state, for states other than FolderError.
func (s *stateTracker) setState(newState folderState) {
	if newState == FolderError {
		panic("must use setError")
	}

	s.mut.Lock()
	defer s.mut.Unlock()

	if newState == s.current {
		return
	}

	defer func() {
		metricFolderState.WithLabelValues(s.folderID).Set(float64(s.current))
	}()

	/* This should hold later...
	if s.current != FolderIdle && (newState == FolderScanning || newState == FolderSyncing) {
		panic("illegal state transition " + s.current.String() + " -> " + newState.String())
	}
	*/

	eventData := map[string]interface{}{
		"folder": s.folderID,
		"to":     newState.String(),
		"from":   s.current.String(),
	}

	if !s.changed.IsZero() {
		eventData["duration"] = time.Since(s.changed).Seconds()
	}

	s.current = newState
	s.changed = time.Now().Truncate(time.Second)

	s.evLogger.Log(events.StateChanged, eventData)
}

// getState returns the current state, the time when it last changed, and the
// current error or nil.
func (s *stateTracker) getState() (current folderState, changed time.Time, err error) {
	s.mut.Lock()
	current, changed, err = s.current, s.changed, s.err
	s.mut.Unlock()
	return
}

// setError sets the folder state to FolderError with the specified error or
// to FolderIdle if the error is nil
func (s *stateTracker) setError(err error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	defer func() {
		metricFolderState.WithLabelValues(s.folderID).Set(float64(s.current))
	}()

	eventData := map[string]interface{}{
		"folder": s.folderID,
		"from":   s.current.String(),
	}

	if err != nil {
		eventData["error"] = err.Error()
		s.current = FolderError
	} else {
		s.current = FolderIdle
	}

	eventData["to"] = s.current.String()

	if !s.changed.IsZero() {
		eventData["duration"] = time.Since(s.changed).Seconds()
	}

	s.err = err
	s.changed = time.Now().Truncate(time.Second)

	s.evLogger.Log(events.StateChanged, eventData)
}
