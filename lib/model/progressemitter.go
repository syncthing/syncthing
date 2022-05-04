// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/model/types"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type ProgressEmitter struct {
	cfg                config.Wrapper
	registry           map[string]map[string]*sharedPullerState // folder: name: puller
	interval           time.Duration
	minBlocks          int
	sentDownloadStates map[protocol.DeviceID]*sentDownloadState // States representing what we've sent to the other peer via DownloadProgress messages.
	connections        map[protocol.DeviceID]protocol.Connection
	foldersByConns     map[protocol.DeviceID][]string
	disabled           bool
	evLogger           events.Logger
	mut                sync.Mutex

	timer *time.Timer
}

type progressUpdate struct {
	conn    protocol.Connection
	folder  string
	updates []protocol.FileDownloadProgressUpdate
}

func (p progressUpdate) send(ctx context.Context) {
	p.conn.DownloadProgress(ctx, p.folder, p.updates)
}

// NewProgressEmitter creates a new progress emitter which emits
// DownloadProgress events every interval.
func NewProgressEmitter(cfg config.Wrapper, evLogger events.Logger) *ProgressEmitter {
	t := &ProgressEmitter{
		cfg:                cfg,
		registry:           make(map[string]map[string]*sharedPullerState),
		timer:              time.NewTimer(time.Millisecond),
		sentDownloadStates: make(map[protocol.DeviceID]*sentDownloadState),
		connections:        make(map[protocol.DeviceID]protocol.Connection),
		foldersByConns:     make(map[protocol.DeviceID][]string),
		evLogger:           evLogger,
		mut:                sync.NewMutex(),
	}

	t.CommitConfiguration(config.Configuration{}, cfg.RawCopy())

	return t
}

// serve starts the progress emitter which starts emitting DownloadProgress
// events as the progress happens.
func (t *ProgressEmitter) Serve(ctx context.Context) error {
	t.cfg.Subscribe(t)
	defer t.cfg.Unsubscribe(t)

	var lastUpdate time.Time
	var lastCount, newCount int
	for {
		select {
		case <-ctx.Done():
			l.Debugln("progress emitter: stopping")
			return nil
		case <-t.timer.C:
			t.mut.Lock()
			l.Debugln("progress emitter: timer - looking after", len(t.registry))

			newLastUpdated := lastUpdate
			newCount = t.lenRegistryLocked()
			var progressUpdates []progressUpdate
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
				progressUpdates = t.computeProgressUpdates()
			} else {
				l.Debugln("progress emitter: nothing new")
			}

			if newCount != 0 {
				t.timer.Reset(t.interval)
			}
			t.mut.Unlock()

			// Do the sending outside of the lock.
			// If these send block, the whole process of reporting progress to others stops, but that's probably fine.
			// It's better to stop this component from working under back-pressure than causing other components that
			// rely on this component to be waiting for locks.
			//
			// This might leave remote peers in some funky state where we are unable the fact that we no longer have
			// something, but there is not much we can do here.
			for _, update := range progressUpdates {
				update.send(ctx)
			}
		}
	}
}

func (t *ProgressEmitter) sendDownloadProgressEventLocked() {
	output := make(events.DownloadProgressEventData)
	for folder, pullers := range t.registry {
		if len(pullers) == 0 {
			continue
		}
		output[folder] = make(map[string]*types.PullerProgress)
		for name, puller := range pullers {
			output[folder][name] = puller.Progress()
		}
	}
	t.evLogger.Log(events.DownloadProgress, output)
	l.Debugf("progress emitter: emitting %#v", output)
}

func (t *ProgressEmitter) computeProgressUpdates() []progressUpdate {
	var progressUpdates []progressUpdate
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
				progressUpdates = append(progressUpdates, progressUpdate{
					conn:    conn,
					folder:  folder,
					updates: updates,
				})
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

	return progressUpdates
}

// CommitConfiguration implements the config.Committer interface
func (t *ProgressEmitter) CommitConfiguration(_, to config.Configuration) bool {
	t.mut.Lock()
	defer t.mut.Unlock()

	newInterval := time.Duration(to.Options.ProgressUpdateIntervalS) * time.Second
	if newInterval > 0 {
		if t.disabled {
			t.disabled = false
			l.Debugln("progress emitter: enabled")
		}
		if t.interval != newInterval {
			t.interval = newInterval
			l.Debugln("progress emitter: updated interval", t.interval)
		}
	} else if !t.disabled {
		t.clearLocked()
		t.disabled = true
		l.Debugln("progress emitter: disabled")
	}
	t.minBlocks = to.Options.TempIndexMinBlocks
	if t.interval < time.Second {
		// can't happen when we're not disabled, but better safe than sorry.
		t.interval = time.Second
	}

	return true
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
				conn.DownloadProgress(context.Background(), folder, updates)
			}
		}
	}
	t.registry = make(map[string]map[string]*sharedPullerState)
	t.sentDownloadStates = make(map[protocol.DeviceID]*sentDownloadState)
	t.connections = make(map[protocol.DeviceID]protocol.Connection)
	t.foldersByConns = make(map[protocol.DeviceID][]string)
}
