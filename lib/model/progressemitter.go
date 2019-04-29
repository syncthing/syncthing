// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type ProgressEmitter struct {
	registry           map[string]map[string]*sharedPullerState // folder: name: puller
	interval           time.Duration
	minBlocks          int
	sentDownloadStates map[protocol.DeviceID]*sentDownloadState // States representing what we've sent to the other peer via DownloadProgress messages.
	connections        map[protocol.DeviceID]protocol.Connection
	foldersByConns     map[protocol.DeviceID][]string
	disabled           bool
	mut                sync.Mutex

	timer *time.Timer

	stop chan struct{}
}

// NewProgressEmitter creates a new progress emitter which emits
// DownloadProgress events every interval.
func NewProgressEmitter(cfg config.Wrapper) *ProgressEmitter {
	t := &ProgressEmitter{
		stop:               make(chan struct{}),
		registry:           make(map[string]map[string]*sharedPullerState),
		timer:              time.NewTimer(time.Millisecond),
		sentDownloadStates: make(map[protocol.DeviceID]*sentDownloadState),
		connections:        make(map[protocol.DeviceID]protocol.Connection),
		foldersByConns:     make(map[protocol.DeviceID][]string),
		mut:                sync.NewMutex(),
	}

	t.CommitConfiguration(config.Configuration{}, cfg.RawCopy())
	cfg.Subscribe(t)

	return t
}

// Serve starts the progress emitter which starts emitting DownloadProgress
// events as the progress happens.
func (t *ProgressEmitter) Serve() {
	var lastUpdate time.Time
	var lastCount, newCount int
	for {
		select {
		case <-t.stop:
			l.Debugln("progress emitter: stopping")
			return
		case <-t.timer.C:
			t.mut.Lock()
			l.Debugln("progress emitter: timer - looking after", len(t.registry))

			newLastUpdated := lastUpdate
			newCount = t.lenRegistryLocked()
			for _, pullers := range t.registry {
				for _, puller := range pullers {
					if updated := puller.Updated(); updated.After(newLastUpdated) {
						newLastUpdated = updated
					}
				}
			}

			if !newLastUpdated.Equal(lastUpdate) || newCount != lastCount {
				lastUpdate = newLastUpdated
				lastCount = newCount
				t.sendDownloadProgressEventLocked()
				if len(t.connections) > 0 {
					t.sendDownloadProgressMessagesLocked()
				}
			} else {
				l.Debugln("progress emitter: nothing new")
			}

			if newCount != 0 {
				t.timer.Reset(t.interval)
			}
			t.mut.Unlock()
		}
	}
}

func (t *ProgressEmitter) sendDownloadProgressEventLocked() {
	output := make(map[string]map[string]*pullerProgress)
	for folder, pullers := range t.registry {
		if len(pullers) == 0 {
			continue
		}
		output[folder] = make(map[string]*pullerProgress)
		for name, puller := range pullers {
			output[folder][name] = puller.Progress()
		}
	}
	events.Default.Log(events.DownloadProgress, output)
	l.Debugf("progress emitter: emitting %#v", output)
}

func (t *ProgressEmitter) sendDownloadProgressMessagesLocked() {
	for id, conn := range t.connections {
		for _, folder := range t.foldersByConns[id] {
			pullers, ok := t.registry[folder]
			if !ok {
				// There's never been any puller registered for this folder yet
				continue
			}

			state, ok := t.sentDownloadStates[id]
			if !ok {
				state = &sentDownloadState{
					folderStates: make(map[string]*sentFolderDownloadState),
				}
				t.sentDownloadStates[id] = state
			}

			activePullers := make([]*sharedPullerState, 0, len(pullers))
			for _, puller := range pullers {
				if puller.folder != folder || puller.file.IsSymlink() || puller.file.IsDirectory() || len(puller.file.Blocks) <= t.minBlocks {
					continue
				}
				activePullers = append(activePullers, puller)
			}

			// For every new puller that hasn't yet been seen, it will send all the blocks the puller has available
			// For every existing puller, it will check for new blocks, and send update for the new blocks only
			// For every puller that we've seen before but is no longer there, we will send a forget message
			updates := state.update(folder, activePullers)

			if len(updates) > 0 {
				conn.DownloadProgress(folder, updates)
			}
		}
	}

	// Clean up sentDownloadStates for devices which we are no longer connected to.
	for id := range t.sentDownloadStates {
		_, ok := t.connections[id]
		if !ok {
			// Null out outstanding entries for device
			delete(t.sentDownloadStates, id)
		}
	}

	// If a folder was unshared from some device, tell it that all temp files
	// are now gone.
	for id, state := range t.sentDownloadStates {
		// For each of the folders that the state is aware of,
		// try to match it with a shared folder we've discovered above,
	nextFolder:
		for _, folder := range state.folders() {
			for _, existingFolder := range t.foldersByConns[id] {
				if existingFolder == folder {
					continue nextFolder
				}
			}

			// If we fail to find that folder, we tell the state to forget about it
			// and return us a list of updates which would clean up the state
			// on the remote end.
			state.cleanup(folder)
			// updates := state.cleanup(folder)
			// if len(updates) > 0 {
			// XXX: Don't send this now, as the only way we've unshared a folder
			// is by breaking the connection and reconnecting, hence sending
			// forget messages for some random folder currently makes no sense.
			// deviceConns[id].DownloadProgress(folder, updates, 0, nil)
			// }
		}
	}
}

// VerifyConfiguration implements the config.Committer interface
func (t *ProgressEmitter) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

// CommitConfiguration implements the config.Committer interface
func (t *ProgressEmitter) CommitConfiguration(from, to config.Configuration) bool {
	t.mut.Lock()
	defer t.mut.Unlock()

	switch {
	case t.disabled && to.Options.ProgressUpdateIntervalS >= 0:
		t.disabled = false
		l.Debugln("progress emitter: enabled")
		fallthrough
	case !t.disabled && from.Options.ProgressUpdateIntervalS != to.Options.ProgressUpdateIntervalS:
		t.interval = time.Duration(to.Options.ProgressUpdateIntervalS) * time.Second
		if t.interval < time.Second {
			t.interval = time.Second
		}
		l.Debugln("progress emitter: updated interval", t.interval)
	case !t.disabled && to.Options.ProgressUpdateIntervalS < 0:
		t.clearLocked()
		t.disabled = true
		l.Debugln("progress emitter: disabled")
	}
	t.minBlocks = to.Options.TempIndexMinBlocks

	return true
}

// Stop stops the emitter.
func (t *ProgressEmitter) Stop() {
	t.stop <- struct{}{}
}

// Register a puller with the emitter which will start broadcasting pullers
// progress.
func (t *ProgressEmitter) Register(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()
	if t.disabled {
		l.Debugln("progress emitter: disabled, skip registering")
		return
	}
	l.Debugln("progress emitter: registering", s.folder, s.file.Name)
	if t.emptyLocked() {
		t.timer.Reset(t.interval)
	}
	if _, ok := t.registry[s.folder]; !ok {
		t.registry[s.folder] = make(map[string]*sharedPullerState)
	}
	t.registry[s.folder][s.file.Name] = s
}

// Deregister a puller which will stop broadcasting pullers state.
func (t *ProgressEmitter) Deregister(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()

	if t.disabled {
		l.Debugln("progress emitter: disabled, skip deregistering")
		return
	}

	l.Debugln("progress emitter: deregistering", s.folder, s.file.Name)
	delete(t.registry[s.folder], s.file.Name)
}

// BytesCompleted returns the number of bytes completed in the given folder.
func (t *ProgressEmitter) BytesCompleted(folder string) (bytes int64) {
	t.mut.Lock()
	defer t.mut.Unlock()

	for _, s := range t.registry[folder] {
		bytes += s.Progress().BytesDone
	}
	l.Debugf("progress emitter: bytes completed for %s: %d", folder, bytes)
	return
}

func (t *ProgressEmitter) String() string {
	return fmt.Sprintf("ProgressEmitter@%p", t)
}

func (t *ProgressEmitter) lenRegistry() int {
	t.mut.Lock()
	defer t.mut.Unlock()
	return t.lenRegistryLocked()
}

func (t *ProgressEmitter) lenRegistryLocked() (out int) {
	for _, pullers := range t.registry {
		out += len(pullers)
	}
	return out
}

func (t *ProgressEmitter) emptyLocked() bool {
	for _, pullers := range t.registry {
		if len(pullers) != 0 {
			return false
		}
	}
	return true
}

func (t *ProgressEmitter) temporaryIndexSubscribe(conn protocol.Connection, folders []string) {
	t.mut.Lock()
	defer t.mut.Unlock()
	t.connections[conn.ID()] = conn
	t.foldersByConns[conn.ID()] = folders
}

func (t *ProgressEmitter) temporaryIndexUnsubscribe(conn protocol.Connection) {
	t.mut.Lock()
	defer t.mut.Unlock()
	delete(t.connections, conn.ID())
	delete(t.foldersByConns, conn.ID())
}

func (t *ProgressEmitter) clearLocked() {
	for id, state := range t.sentDownloadStates {
		conn, ok := t.connections[id]
		if !ok {
			continue
		}
		for _, folder := range state.folders() {
			if updates := state.cleanup(folder); len(updates) > 0 {
				conn.DownloadProgress(folder, updates)
			}
		}
	}
	t.registry = make(map[string]map[string]*sharedPullerState)
	t.sentDownloadStates = make(map[protocol.DeviceID]*sentDownloadState)
	t.connections = make(map[protocol.DeviceID]protocol.Connection)
	t.foldersByConns = make(map[protocol.DeviceID][]string)
}
