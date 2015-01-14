// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"testing"
	"time"

	"github.com/syncthing/protocol"
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

	s.copyDone(protocol.BlockInfo{})

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.copiedFromOrigin()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.pullStarted()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.pullDone(protocol.BlockInfo{})

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	p.Deregister(&s)

	expectEvent(w, t, 0)
	expectTimeout(w, t)

}
