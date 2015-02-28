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
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/events"
)

var timeout = 10 * time.Millisecond

func expectEvent(w *events.Subscription, t *testing.T, size int) {
	event, err := w.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if event.Type != events.DownloadProgress {
		t.Fatal("Unexpected event:", event)
	}
	data := event.Data.(map[string]map[string]*pullerProgress)
	if len(data) != size {
		t.Fatal("Unexpected event data size:", data)
	}
}

func expectTimeout(w *events.Subscription, t *testing.T) {
	_, err := w.Poll(timeout)
	if err != events.ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestProgressEmitter(t *testing.T) {
	w := events.Default.Subscribe(events.DownloadProgress)

	c := config.Wrap("/tmp/test", config.Configuration{})
	c.SetOptions(config.OptionsConfiguration{
		ProgressUpdateIntervalS: 0,
	})

	p := NewProgressEmitter(c)
	go p.Serve()

	expectTimeout(w, t)

	s := sharedPullerState{}
	p.Register(&s)

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.copyDone()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.copiedFromOrigin()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.pullStarted()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.pullDone()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	p.Deregister(&s)

	expectEvent(w, t, 0)
	expectTimeout(w, t)

}
