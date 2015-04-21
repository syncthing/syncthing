// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/events"
)

type progressEmitter struct {
	tracker *progressTracker
	last    time.Time

	interval time.Duration
	mut      sync.RWMutex

	timer *time.Timer

	stop chan struct{}
}

// Creates a new progress emitter which emits DownloadProgress events every
// interval.
func newProgressEmitter(tracker *progressTracker, cfg *config.Wrapper) *progressEmitter {
	t := &progressEmitter{
		tracker: tracker,
		stop:    make(chan struct{}),
		last:    time.Time{},
		timer:   time.NewTimer(time.Millisecond),
	}
	t.Changed(cfg.Raw())
	cfg.Subscribe(t)
	return t
}

// Starts progress emitter which starts emitting DownloadProgress events as
// the progress happens.
func (t *progressEmitter) Serve() {
	for {
		select {
		case <-t.stop:
			if debug {
				l.Debugln("progress emitter: stopping")
			}
			return
		case <-t.timer.C:
			lastChange := t.tracker.lastChanged()
			if t.last.Before(lastChange) {
				// XXX: Race condition between lastChanged() and getActivePullers()
				// But the worst thing that could happen is that we would emit
				// DownloadProgress event for the same data twice.
				t.last = lastChange
				pullers := t.tracker.getActivePullers()

				output := make(map[string]map[string]*pullerProgress)
				for _, puller := range pullers {
					if output[puller.folder] == nil {
						output[puller.folder] = make(map[string]*pullerProgress)
					}
					output[puller.folder][puller.file.Name] = puller.Progress()
				}

				events.Default.Log(events.DownloadProgress, output)
				if debug {
					l.Debugf("progress emitter: emitting %#v", output)
				}

			} else if debug {
				l.Debugln("progress emitter: nothing new")
			}

			t.mut.RLock()
			t.timer.Reset(t.interval)
			t.mut.RUnlock()
		}
	}
}

// Interface method to handle configuration changes
func (t *progressEmitter) Changed(cfg config.Configuration) error {
	t.mut.Lock()
	t.interval = time.Duration(cfg.Options.ProgressUpdateIntervalS) * time.Second
	if debug {
		l.Debugln("progress emitter: updated interval", t.interval)
	}
	t.mut.Unlock()
	return nil
}

// Stops the emitter.
func (t *progressEmitter) Stop() {
	t.stop <- struct{}{}
}
