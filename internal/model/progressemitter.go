// Copyright (C) 2014 The Syncthing Authors.
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
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/events"
)

type ProgressEmitter struct {
	registry map[string]*sharedPullerState
	interval time.Duration
	last     map[string]map[string]*pullerProgress
	mut      sync.Mutex

	timer *time.Timer

	stop chan struct{}
}

// Creates a new progress emitter which emits DownloadProgress events every
// interval.
func NewProgressEmitter(cfg *config.Wrapper) *ProgressEmitter {
	t := &ProgressEmitter{
		stop:     make(chan struct{}),
		registry: make(map[string]*sharedPullerState),
		last:     make(map[string]map[string]*pullerProgress),
		timer:    time.NewTimer(time.Millisecond),
	}
	t.Changed(cfg.Raw())
	cfg.Subscribe(t)
	return t
}

// Starts progress emitter which starts emitting DownloadProgress events as
// the progress happens.
func (t *ProgressEmitter) Serve() {
	for {
		select {
		case <-t.stop:
			if debug {
				l.Debugln("progress emitter: stopping")
			}
			return
		case <-t.timer.C:
			t.mut.Lock()
			if debug {
				l.Debugln("progress emitter: timer - looking after", len(t.registry))
			}
			output := make(map[string]map[string]*pullerProgress)
			for _, puller := range t.registry {
				if output[puller.folder] == nil {
					output[puller.folder] = make(map[string]*pullerProgress)
				}
				output[puller.folder][puller.file.Name] = puller.Progress()
			}
			if !reflect.DeepEqual(t.last, output) {
				events.Default.Log(events.DownloadProgress, output)
				t.last = output
				if debug {
					l.Debugf("progress emitter: emitting %#v", output)
				}
			} else if debug {
				l.Debugln("progress emitter: nothing new")
			}
			if len(t.registry) != 0 {
				t.timer.Reset(t.interval)
			}
			t.mut.Unlock()
		}
	}
}

// Interface method to handle configuration changes
func (t *ProgressEmitter) Changed(cfg config.Configuration) error {
	t.mut.Lock()
	defer t.mut.Unlock()

	t.interval = time.Duration(cfg.Options.ProgressUpdateIntervalS) * time.Second
	if debug {
		l.Debugln("progress emitter: updated interval", t.interval)
	}
	return nil
}

// Stops the emitter.
func (t *ProgressEmitter) Stop() {
	t.stop <- struct{}{}
}

// Register a puller with the emitter which will start broadcasting pullers
// progress.
func (t *ProgressEmitter) Register(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()
	if debug {
		l.Debugln("progress emitter: registering", s.folder, s.file.Name)
	}
	if len(t.registry) == 0 {
		t.timer.Reset(t.interval)
	}
	t.registry[filepath.Join(s.folder, s.file.Name)] = s
}

// Deregister a puller which will stop boardcasting pullers state.
func (t *ProgressEmitter) Deregister(s *sharedPullerState) {
	t.mut.Lock()
	defer t.mut.Unlock()
	if debug {
		l.Debugln("progress emitter: deregistering", s.folder, s.file.Name)
	}
	delete(t.registry, filepath.Join(s.folder, s.file.Name))
}

// Returns number of bytes completed in the given folder.
func (t *ProgressEmitter) BytesCompleted(folder string) (bytes int64) {
	t.mut.Lock()
	defer t.mut.Unlock()

	for _, s := range t.registry {
		if s.folder == folder {
			bytes += s.Progress().BytesDone
		}
	}
	if debug {
		l.Debugf("progress emitter: bytes completed for %s: %d", folder, bytes)
	}
	return
}
