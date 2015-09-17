// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/sync"
)

type ProgressEmitter struct {
	registry   map[string]*sharedPullerState
	interval   time.Duration
	lastUpdate time.Time
	mut        sync.Mutex

	timer *time.Timer

	stop chan struct{}
}

// NewProgressEmitter creates a new progress emitter which emits
// DownloadProgress events every interval.
func NewProgressEmitter(cfg *config.Wrapper) *ProgressEmitter {
	t := &ProgressEmitter{
		stop:     make(chan struct{}),
		registry: make(map[string]*sharedPullerState),
		timer:    time.NewTimer(time.Millisecond),
		mut:      sync.NewMutex(),
	}

	t.CommitConfiguration(config.Configuration{}, cfg.Raw())
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

// VerifyConfiguration implements the config.Committer interface
func (t *ProgressEmitter) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

// CommitConfiguration implements the config.Committer interface
func (t *ProgressEmitter) CommitConfiguration(from, to config.Configuration) bool {
	t.mut.Lock()
	defer t.mut.Unlock()

	t.interval = time.Duration(to.Options.ProgressUpdateIntervalS) * time.Second
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
	t.registry[filepath.Join(s.folder, s.file.Name)] = s
}

// Deregister a puller which will stop broadcasting pullers state.
func (t *ProgressEmitter) Deregister(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()

	l.Debugln("progress emitter: deregistering", s.folder, s.file.Name)

	delete(t.registry, filepath.Join(s.folder, s.file.Name))
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
