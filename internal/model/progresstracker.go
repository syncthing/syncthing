// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"sync"
	"time"

	"github.com/syncthing/protocol"
)

type progressTracker struct {
	// Ones which have been constructed, but haven't yet opened a temp file,
	// probably next in the channel
	pendingPullers []*sharedPullerState
	// Currently in progress
	activePullers []*sharedPullerState
	lastCopy      time.Time
	lastPull      time.Time
	lastChange    time.Time
	mut           sync.RWMutex
}

func newProgressTracker() *progressTracker {
	return &progressTracker{
		pendingPullers: make([]*sharedPullerState, 0),
		activePullers:  make([]*sharedPullerState, 0),
	}
}

func (t *progressTracker) newSharedPullerState(file protocol.FileInfo, folder, tempName, realName string, blocks, reused int, ignorePerms bool, version protocol.Vector, availableBlocks []protocol.BlockInfo) *sharedPullerState {
	s := &sharedPullerState{
		progressTracker: t,
		file:            file,
		folder:          folder,
		tempName:        tempName,
		realName:        realName,
		copyTotal:       blocks,
		copyNeeded:      blocks,
		reused:          reused,
		ignorePerms:     ignorePerms,
		version:         version,
		available:       availableBlocks,
	}
	t.mut.Lock()
	t.pendingPullers = append(t.pendingPullers, s)
	t.mut.Unlock()
	return s
}

// Marks a puller as started
func (t *progressTracker) start(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()
	for i, state := range t.pendingPullers {
		if state.folder == s.folder && state.file.Name == s.file.Name {
			t.lastChange = time.Now()
			last := len(t.pendingPullers) - 1

			t.pendingPullers[i] = t.pendingPullers[last]
			t.pendingPullers[last] = nil
			t.pendingPullers = t.pendingPullers[:last]

			t.activePullers = append(t.activePullers, s)

			if debug {
				l.Debugln("progress tracker: starting", s.folder, s.file.Name)
			}
			return
		}
	}
	panic("Puller state " + s.file.Name + " " + s.folder + " not found")
}

// Stops tracking the given puller.
func (t *progressTracker) finished(s *sharedPullerState) {
	t.mut.Lock()

	// Create a new slice as someone might be iterating over the existing one.

	pullers := make([]*sharedPullerState, 0, len(t.activePullers)-1)
	found := false

	for _, state := range t.activePullers {
		if state.folder != s.folder || state.file.Name != s.file.Name {
			pullers = append(pullers, state)
		} else {
			found = true
		}
	}

	if !found {
		panic("Puller state " + s.file.Name + " " + s.folder + " not found")
	}

	if debug {
		l.Debugln("progress tracker: finished", s.folder, s.file.Name)
	}
	t.lastChange = time.Now()
	t.activePullers = pullers
	t.mut.Unlock()
}

// Called by sharedPullerState to notify that a block has been copied.
func (t *progressTracker) copied() {
	t.mut.Lock()
	now := time.Now()
	t.lastCopy = now
	t.lastChange = now
	t.mut.Unlock()
}

// Called by sharedPullerState to notify that a block has been pulled.
func (t *progressTracker) pulled() {
	t.mut.Lock()
	now := time.Now()
	t.lastPull = now
	t.lastChange = now
	t.mut.Unlock()
}

// Called by sharedPullerState to notify that something has changed,
// but it's not something which changes the list of available blocks.
// progressEmitter is interested in events such as copiedFromOrigin,
// pullStarted, and others.
func (t *progressTracker) progressed() {
	t.mut.Lock()
	t.lastChange = time.Now()
	t.mut.Unlock()
}

// Gets all active pullers for all folders
func (t *progressTracker) getActivePullers() []*sharedPullerState {
	t.mut.RLock()
	defer t.mut.RUnlock()
	return t.activePullers
}

// Gets active pullers for a given folder
func (t *progressTracker) getActivePullersForFolder(folder string) []*sharedPullerState {
	states := make([]*sharedPullerState, 0, 0)
	t.mut.RLock()
	for _, state := range t.activePullers {
		if state.folder == folder {
			states = append(states, state)
		}
	}
	t.mut.RUnlock()
	return states
}

// Get's the shared puller state for the given file in the given folder.
func (t *progressTracker) getActivePullerState(folder, file string) *sharedPullerState {
	t.mut.RLock()
	defer t.mut.RUnlock()

	for _, state := range t.activePullers {
		if state.folder == folder && state.file.Name == file {
			return state
		}
	}
	return nil
}

// Called by external parties to get the time when the last block was pulled.
func (t *progressTracker) lastPulled() time.Time {
	t.mut.RLock()
	defer t.mut.RUnlock()
	return t.lastPull
}

// Called by external parties to get the time when the last block was copied.
func (t *progressTracker) lastCopied() time.Time {
	t.mut.RLock()
	defer t.mut.RUnlock()
	return t.lastCopy
}

// Called by external parties to get the time when the last change has happened.
func (t *progressTracker) lastChanged() time.Time {
	t.mut.RLock()
	defer t.mut.RUnlock()
	return t.lastChange
}
