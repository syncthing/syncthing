// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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
	FolderSyncing
	FolderError
)

func (s folderState) String() string {
	switch s {
	case FolderIdle:
		return "idle"
	case FolderScanning:
		return "scanning"
	case FolderSyncing:
		return "syncing"
	case FolderError:
		return "error"
	default:
		return "unknown"
	}
}

type stateTracker struct {
	folder string

	mut     sync.Mutex
	current folderState
	err     error
	changed time.Time
}

// setState sets the new folder state, for states other than FolderError.
func (s *stateTracker) setState(newState folderState) {
	if newState == FolderError {
		panic("must use setError")
	}

	s.mut.Lock()
	if newState != s.current {
		/* This should hold later...
		if s.current != FolderIdle && (newState == FolderScanning || newState == FolderSyncing) {
			panic("illegal state transition " + s.current.String() + " -> " + newState.String())
		}
		*/

		eventData := map[string]interface{}{
			"folder": s.folder,
			"to":     newState.String(),
			"from":   s.current.String(),
		}

		if !s.changed.IsZero() {
			eventData["duration"] = time.Since(s.changed).Seconds()
		}

		s.current = newState
		s.changed = time.Now()

		events.Default.Log(events.StateChanged, eventData)
	}
	s.mut.Unlock()
}

// getState returns the current state, the time when it last changed, and the
// current error or nil.
func (s *stateTracker) getState() (current folderState, changed time.Time, err error) {
	s.mut.Lock()
	current, changed, err = s.current, s.changed, s.err
	s.mut.Unlock()
	return
}

// setError sets the folder state to FolderError with the specified error.
func (s *stateTracker) setError(err error) {
	s.mut.Lock()
	if s.current != FolderError || s.err.Error() != err.Error() {
		eventData := map[string]interface{}{
			"folder": s.folder,
			"to":     FolderError.String(),
			"from":   s.current.String(),
			"error":  err.Error(),
		}

		if !s.changed.IsZero() {
			eventData["duration"] = time.Since(s.changed).Seconds()
		}

		s.current = FolderError
		s.err = err
		s.changed = time.Now()

		events.Default.Log(events.StateChanged, eventData)
	}
	s.mut.Unlock()
}

// clearError sets the folder state to FolderIdle and clears the error
func (s *stateTracker) clearError() {
	s.mut.Lock()
	if s.current == FolderError {
		eventData := map[string]interface{}{
			"folder": s.folder,
			"to":     FolderIdle.String(),
			"from":   s.current.String(),
		}

		if !s.changed.IsZero() {
			eventData["duration"] = time.Since(s.changed).Seconds()
		}

		s.current = FolderIdle
		s.err = nil
		s.changed = time.Now()

		events.Default.Log(events.StateChanged, eventData)
	}
	s.mut.Unlock()
}
