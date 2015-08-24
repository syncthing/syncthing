// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package events_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/events"
)

const timeout = 100 * time.Millisecond

func TestNewLogger(t *testing.T) {
	l := events.NewLogger()
	if l == nil {
		t.Fatal("Unexpected nil Logger")
	}
}

func TestSubscriber(t *testing.T) {
	l := events.NewLogger()
	s := l.Subscribe(0)
	defer l.Unsubscribe(s)
	if s == nil {
		t.Fatal("Unexpected nil Subscription")
	}
}

func TestTimeout(t *testing.T) {
	l := events.NewLogger()
	s := l.Subscribe(0)
	defer l.Unsubscribe(s)
	_, err := s.Poll(timeout)
	if err != events.ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestEventBeforeSubscribe(t *testing.T) {
	l := events.NewLogger()

	l.Log(events.DeviceConnected, "foo")
	s := l.Subscribe(0)
	defer l.Unsubscribe(s)

	_, err := s.Poll(timeout)
	if err != events.ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestEventAfterSubscribe(t *testing.T) {
	l := events.NewLogger()

	s := l.Subscribe(events.AllEvents)
	defer l.Unsubscribe(s)
	l.Log(events.DeviceConnected, "foo")

	ev, err := s.Poll(timeout)

	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.Type != events.DeviceConnected {
		t.Error("Incorrect event type", ev.Type)
	}
	switch v := ev.Data.(type) {
	case string:
		if v != "foo" {
			t.Error("Incorrect Data string", v)
		}
	default:
		t.Errorf("Incorrect Data type %#v", v)
	}
}

func TestEventAfterSubscribeIgnoreMask(t *testing.T) {
	l := events.NewLogger()

	s := l.Subscribe(events.DeviceDisconnected)
	defer l.Unsubscribe(s)
	l.Log(events.DeviceConnected, "foo")

	_, err := s.Poll(timeout)
	if err != events.ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestBufferOverflow(t *testing.T) {
	l := events.NewLogger()

	s := l.Subscribe(events.AllEvents)
	defer l.Unsubscribe(s)

	t0 := time.Now()
	for i := 0; i < events.BufferSize*2; i++ {
		l.Log(events.DeviceConnected, "foo")
	}
	if time.Since(t0) > timeout {
		t.Fatalf("Logging took too long")
	}
}

func TestUnsubscribe(t *testing.T) {
	l := events.NewLogger()

	s := l.Subscribe(events.AllEvents)
	l.Log(events.DeviceConnected, "foo")

	_, err := s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	l.Unsubscribe(s)
	l.Log(events.DeviceConnected, "foo")

	_, err = s.Poll(timeout)
	if err != events.ErrClosed {
		t.Fatal("Unexpected non-Closed error:", err)
	}
}

func TestIDs(t *testing.T) {
	l := events.NewLogger()

	s := l.Subscribe(events.AllEvents)
	defer l.Unsubscribe(s)
	l.Log(events.DeviceConnected, "foo")
	l.Log(events.DeviceConnected, "bar")

	ev, err := s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.Data.(string) != "foo" {
		t.Fatal("Incorrect event:", ev)
	}
	id := ev.ID

	ev, err = s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.Data.(string) != "bar" {
		t.Fatal("Incorrect event:", ev)
	}
	if !(ev.ID > id) {
		t.Fatalf("ID not incremented (%d !> %d)", ev.ID, id)
	}
}

func TestBufferedSub(t *testing.T) {
	l := events.NewLogger()

	s := l.Subscribe(events.AllEvents)
	defer l.Unsubscribe(s)
	bs := events.NewBufferedSubscription(s, 10*events.BufferSize)

	go func() {
		for i := 0; i < 10*events.BufferSize; i++ {
			l.Log(events.DeviceConnected, fmt.Sprintf("event-%d", i))
			if i%30 == 0 {
				// Give the buffer routine time to pick up the events
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()

	recv := 0
	for recv < 10*events.BufferSize {
		evs := bs.Since(recv, nil)
		for _, ev := range evs {
			if ev.ID != recv+1 {
				t.Fatalf("Incorrect ID; %d != %d", ev.ID, recv+1)
			}
			recv = ev.ID
		}
	}

}
