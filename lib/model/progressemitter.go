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
	registry           map[string]*sharedPullerState
	interval           time.Duration
	minBlocks          int
	sentDownloadStates map[protocol.DeviceID]*sentDownloadState // States representing what we've sent to the other peer via DownloadProgress messages.
	connections        map[string][]protocol.Connection
	mut                sync.Mutex

	timer *time.Timer

	stop chan struct{}
}

// NewProgressEmitter creates a new progress emitter which emits
// DownloadProgress events every interval.
func NewProgressEmitter(cfg *config.Wrapper) *ProgressEmitter {
	t := &ProgressEmitter{
		stop:               make(chan struct{}),
		registry:           make(map[string]*sharedPullerState),
		timer:              time.NewTimer(time.Millisecond),
		sentDownloadStates: make(map[protocol.DeviceID]*sentDownloadState),
		connections:        make(map[string][]protocol.Connection),
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
			newCount = len(t.registry)
			for _, puller := range t.registry {
				updated := puller.Updated()
				if updated.After(newLastUpdated) {
					newLastUpdated = updated
				}
			}

			if !newLastUpdated.Equal(lastUpdate) || newCount != lastCount {
				lastUpdate = newLastUpdated
				lastCount = newCount
				t.sendDownloadProgressEvent()
				if len(t.connections) > 0 {
					t.sendDownloadProgressMessages()
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

func (t *ProgressEmitter) sendDownloadProgressEvent() {
	// registry lock already held
	output := make(map[string]map[string]*pullerProgress)
	for _, puller := range t.registry {
		if output[puller.folder] == nil {
			output[puller.folder] = make(map[string]*pullerProgress)
		}
		output[puller.folder][puller.file.Name] = puller.Progress()
	}
	events.Default.Log(events.DownloadProgress, output)
	l.Debugf("progress emitter: emitting %#v", output)
}

func (t *ProgressEmitter) sendDownloadProgressMessages() {
	// registry lock already held
	sharedFolders := make(map[protocol.DeviceID][]string)
	deviceConns := make(map[protocol.DeviceID]protocol.Connection)
	subscribers := t.connections
	for folder, conns := range subscribers {
		for _, conn := range conns {
			id := conn.ID()

			deviceConns[id] = conn
			sharedFolders[id] = append(sharedFolders[id], folder)

			state, ok := t.sentDownloadStates[id]
			if !ok {
				state = &sentDownloadState{
					folderStates: make(map[string]*sentFolderDownloadState),
				}
				t.sentDownloadStates[id] = state
			}

			var activePullers []*sharedPullerState
			for _, puller := range t.registry {
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
		_, ok := deviceConns[id]
		if !ok {
			// Null out outstanding entries for device
			delete(t.sentDownloadStates, id)
		}
	}

	// If a folder was unshared from some device, tell it that all temp files
	// are now gone.
	for id, sharedDeviceFolders := range sharedFolders {
		state := t.sentDownloadStates[id]
	nextFolder:
		// For each of the folders that the state is aware of,
		// try to match it with a shared folder we've discovered above,
		for _, folder := range state.folders() {
			for _, existingFolder := range sharedDeviceFolders {
				if existingFolder == folder {
					continue nextFolder
				}
			}

			// If we fail to find that folder, we tell the state to forget about it
			// and return us a list of updates which would clean up the state
			// on the remote end.
			updates := state.cleanup(folder)
			if len(updates) > 0 {
				// XXX: Don't send this now, as the only way we've unshared a folder
				// is by breaking the connection and reconnecting, hence sending
				// forget messages for some random folder currently makes no sense.
				// deviceConns[id].DownloadProgress(folder, updates, 0, nil)
			}
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

	t.interval = time.Duration(to.Options.ProgressUpdateIntervalS) * time.Second
	if t.interval < time.Second {
		t.interval = time.Second
	}
	t.minBlocks = to.Options.TempIndexMinBlocks
	l.Debugln("progress emitter: updated interval", t.interval)

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
	l.Debugln("progress emitter: registering", s.folder, s.file.Name)
	if len(t.registry) == 0 {
		t.timer.Reset(t.interval)
	}
	// Separate the folder ID (arbitrary string) and the file name by "//"
	// because it never appears in a valid file name.
	t.registry[s.folder+"//"+s.file.Name] = s
}

// Deregister a puller which will stop broadcasting pullers state.
func (t *ProgressEmitter) Deregister(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()

	l.Debugln("progress emitter: deregistering", s.folder, s.file.Name)

	delete(t.registry, s.folder+"//"+s.file.Name)
}

// BytesCompleted returns the number of bytes completed in the given folder.
func (t *ProgressEmitter) BytesCompleted(folder string) (bytes int64) {
	t.mut.Lock()
	defer t.mut.Unlock()

	for _, s := range t.registry {
		if s.folder == folder {
			bytes += s.Progress().BytesDone
		}
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
	return len(t.registry)
}

func (t *ProgressEmitter) temporaryIndexSubscribe(conn protocol.Connection, folders []string) {
	t.mut.Lock()
	for _, folder := range folders {
		t.connections[folder] = append(t.connections[folder], conn)
	}
	t.mut.Unlock()
}

func (t *ProgressEmitter) temporaryIndexUnsubscribe(conn protocol.Connection) {
	t.mut.Lock()
	left := make(map[string][]protocol.Connection, len(t.connections))
	for folder, conns := range t.connections {
		connsLeft := connsWithout(conns, conn)
		if len(connsLeft) > 0 {
			left[folder] = connsLeft
		}
	}
	t.connections = left
	t.mut.Unlock()
}

func connsWithout(conns []protocol.Connection, conn protocol.Connection) []protocol.Connection {
	if len(conns) == 0 {
		return nil
	}

	newConns := make([]protocol.Connection, 0, len(conns)-1)
	for _, existingConn := range conns {
		if existingConn != conn {
			newConns = append(newConns, existingConn)
		}
	}
	return newConns
}
