// Copyright (C) 2015 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package model

import (
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/events"
)

type folderState int

const (
	FolderIdle folderState = iota
	FolderScanning
	FolderSyncing
	FolderCleaning
)

func (s folderState) String() string {
	switch s {
	case FolderIdle:
		return "idle"
	case FolderScanning:
		return "scanning"
	case FolderCleaning:
		return "cleaning"
	case FolderSyncing:
		return "syncing"
	default:
		return "unknown"
	}
}

type stateTracker struct {
	folder string

	mut     sync.Mutex
	current folderState
	changed time.Time
}

func (s *stateTracker) setState(newState folderState) {
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

func (s *stateTracker) getState() (current folderState, changed time.Time) {
	s.mut.Lock()
	current, changed = s.current, s.changed
	s.mut.Unlock()
	return
}
