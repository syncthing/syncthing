// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"strings"
	"testing"

	"github.com/thejerf/suture/v4"
)

func TestServiceMap(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := suture.NewSimple("TestServiceMap")
	sup.ServeBackground(ctx)

	t.Run("SimpleAddRemove", func(t *testing.T) {
		t.Parallel()

		sm := newServiceMap[string, *dummyService]()
		sup.Add(sm)

		// Add two services. They should start.

		d1 := newDummyService()
		d2 := newDummyService()

		sm.Add("d1", d1)
		sm.Add("d2", d2)

		<-d1.started
		<-d2.started

		// Remove them. They should stop.

		if !sm.Remove("d1") {
			t.Errorf("Remove failed")
		}
		if !sm.Remove("d2") {
			t.Errorf("Remove failed")
		}

		<-d1.stopped
		<-d2.stopped
	})

	t.Run("OverwriteImpliesRemove", func(t *testing.T) {
		t.Parallel()

		sm := newServiceMap[string, *dummyService]()
		sup.Add(sm)

		d1 := newDummyService()
		d2 := newDummyService()

		// Add d1, it should start.

		sm.Add("k", d1)
		<-d1.started

		// Add d2, with the same key. The previous one should stop as we're
		// doing a replace.

		sm.Add("k", d2)
		<-d1.stopped
		<-d2.started

		if !sm.Remove("k") {
			t.Errorf("Remove failed")
		}

		<-d2.stopped
	})

	t.Run("IterateWithRemoveAndWait", func(t *testing.T) {
		t.Parallel()

		sm := newServiceMap[string, *dummyService]()
		sup.Add(sm)

		// Add four services.

		d1 := newDummyService()
		d2 := newDummyService()
		d3 := newDummyService()
		d4 := newDummyService()

		sm.Add("keep1", d1)
		sm.Add("remove2", d2)
		sm.Add("keep3", d3)
		sm.Add("remove4", d4)

		<-d1.started
		<-d2.started
		<-d3.started
		<-d4.started

		// Remove two of them from within the iterator.

		sm.Each(func(k string, v *dummyService) {
			if strings.HasPrefix(k, "remove") {
				sm.RemoveAndWait(k, 0)
			}
		})

		// They should have stopped.

		<-d2.stopped
		<-d4.stopped

		// They should not be in the map anymore.

		if _, ok := sm.Get("remove2"); ok {
			t.Errorf("Service still in map")
		}
		if _, ok := sm.Get("remove4"); ok {
			t.Errorf("Service still in map")
		}

		// The other two should still be running.

		if _, ok := sm.Get("keep1"); !ok {
			t.Errorf("Service not in map")
		}
		if _, ok := sm.Get("keep3"); !ok {
			t.Errorf("Service not in map")
		}
	})
}

type dummyService struct {
	started chan struct{}
	stopped chan struct{}
}

func newDummyService() *dummyService {
	return &dummyService{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (d *dummyService) Serve(ctx context.Context) error {
	close(d.started)
	defer close(d.stopped)
	<-ctx.Done()
	return nil
}
