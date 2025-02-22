// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

const timeout = time.Second

func init() {
	runningTests = true
}

func TestNewLogger(t *testing.T) {
	l := NewLogger()
	if l == nil {
		t.Fatal("Unexpected nil Logger")
	}
}

func setupLogger() (Logger, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	l := NewLogger()
	go l.Serve(ctx)
	return l, cancel
}

func TestSubscriber(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(0)
	defer s.Unsubscribe()
	if s == nil {
		t.Fatal("Unexpected nil Subscription")
	}
}

func TestTimeout(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(0)
	defer s.Unsubscribe()
	_, err := s.Poll(timeout)
	if err != ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestEventBeforeSubscribe(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	l.Log(DeviceConnected, "foo")
	s := l.Subscribe(0)
	defer s.Unsubscribe()

	_, err := s.Poll(timeout)
	if err != ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestEventAfterSubscribe(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	defer s.Unsubscribe()
	l.Log(DeviceConnected, "foo")

	ev, err := s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.Type != DeviceConnected {
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
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(DeviceDisconnected)
	defer s.Unsubscribe()
	l.Log(DeviceConnected, "foo")

	_, err := s.Poll(timeout)
	if err != ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err)
	}
}

func TestBufferOverflow(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	defer s.Unsubscribe()

	// The first BufferSize events will be logged pretty much
	// instantaneously. The next BufferSize events will each block for up to
	// 15ms, plus overhead from race detector and thread scheduling latency
	// etc. This latency can sometimes be significant and is incurred for
	// each call. We just verify that the whole test completes in a
	// reasonable time, taking no more than 15 seconds in total.

	t0 := time.Now()
	const nEvents = BufferSize * 2
	for i := 0; i < nEvents; i++ {
		l.Log(DeviceConnected, "foo")
	}
	if d := time.Since(t0); d > 15*time.Second {
		t.Fatal("Logging took too long,", d, "avg", d/nEvents, "expected <", eventLogTimeout)
	}
}

func TestUnsubscribe(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	l.Log(DeviceConnected, "foo")

	_, err := s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	s.Unsubscribe()
	l.Log(DeviceConnected, "foo")

	_, err = s.Poll(timeout)
	if err != ErrClosed {
		t.Fatal("Unexpected non-Closed error:", err)
	}
}

func TestGlobalIDs(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	defer s.Unsubscribe()
	l.Log(DeviceConnected, "foo")
	l.Subscribe(AllEvents)
	l.Log(DeviceConnected, "bar")

	ev, err := s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.Data.(string) != "foo" {
		t.Fatal("Incorrect event:", ev)
	}
	id := ev.GlobalID

	ev, err = s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.Data.(string) != "bar" {
		t.Fatal("Incorrect event:", ev)
	}
	if ev.GlobalID != id+1 {
		t.Fatalf("ID not incremented (%d != %d)", ev.GlobalID, id+1)
	}
}

func TestSubscriptionIDs(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(DeviceConnected)
	defer s.Unsubscribe()

	l.Log(DeviceDisconnected, "a")
	l.Log(DeviceConnected, "b")
	l.Log(DeviceConnected, "c")
	l.Log(DeviceDisconnected, "d")

	ev, err := s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}

	if ev.GlobalID != 2 {
		t.Fatal("Incorrect GlobalID:", ev.GlobalID)
	}
	if ev.SubscriptionID != 1 {
		t.Fatal("Incorrect SubscriptionID:", ev.SubscriptionID)
	}

	ev, err = s.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if ev.GlobalID != 3 {
		t.Fatal("Incorrect GlobalID:", ev.GlobalID)
	}
	if ev.SubscriptionID != 2 {
		t.Fatal("Incorrect SubscriptionID:", ev.SubscriptionID)
	}

	ev, err = s.Poll(timeout)
	if err != ErrTimeout {
		t.Fatal("Unexpected error:", err)
	}
}

func TestBufferedSub(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	defer s.Unsubscribe()
	bs := NewBufferedSubscription(s, 10*BufferSize)

	go func() {
		for i := 0; i < 10*BufferSize; i++ {
			l.Log(DeviceConnected, fmt.Sprintf("event-%d", i))
			if i%30 == 0 {
				// Give the buffer routine time to pick up the events
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()

	recv := 0
	for recv < 10*BufferSize {
		evs := bs.Since(recv, nil, time.Minute)
		for _, ev := range evs {
			if ev.GlobalID != recv+1 {
				t.Fatalf("Incorrect ID; %d != %d", ev.GlobalID, recv+1)
			}
			recv = ev.GlobalID
		}
	}
}

func BenchmarkBufferedSub(b *testing.B) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	defer s.Unsubscribe()
	bufferSize := BufferSize
	bs := NewBufferedSubscription(s, bufferSize)

	// The coord channel paces the sender according to the receiver,
	// ensuring that no events are dropped. The benchmark measures sending +
	// receiving + synchronization overhead.

	coord := make(chan struct{}, bufferSize)
	for i := 0; i < bufferSize-1; i++ {
		coord <- struct{}{}
	}

	// Receive the events
	done := make(chan error)
	go func() {
		recv := 0
		var evs []Event
		for i := 0; i < b.N; {
			evs = bs.Since(recv, evs[:0], time.Minute)
			for _, ev := range evs {
				if ev.GlobalID != recv+1 {
					done <- fmt.Errorf("skipped event %v %v", ev.GlobalID, recv)
					return
				}
				recv = ev.GlobalID
				coord <- struct{}{}
			}
			i += len(evs)
		}
		done <- nil
	}()

	// Send the events
	eventData := map[string]string{
		"foo":   "bar",
		"other": "data",
		"and":   "something else",
	}
	for i := 0; i < b.N; i++ {
		l.Log(DeviceConnected, eventData)
		<-coord
	}

	if err := <-done; err != nil {
		b.Error(err)
	}
	b.ReportAllocs()
}

func TestSinceUsesSubscriptionId(t *testing.T) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(DeviceConnected)
	defer s.Unsubscribe()
	bs := NewBufferedSubscription(s, 10*BufferSize)

	l.Log(DeviceConnected, "a") // SubscriptionID = 1
	l.Log(DeviceDisconnected, "b")
	l.Log(DeviceDisconnected, "c")
	l.Log(DeviceConnected, "d") // SubscriptionID = 2

	// We need to loop for the events, as they may not all have been
	// delivered to the buffered subscription when we get here.
	t0 := time.Now()
	for time.Since(t0) < time.Second {
		events := bs.Since(0, nil, time.Minute)
		if len(events) == 2 {
			break
		}
		if len(events) > 2 {
			t.Fatal("Incorrect number of events:", len(events))
		}
	}

	events := bs.Since(1, nil, time.Minute)
	if len(events) != 1 {
		t.Fatal("Incorrect number of events:", len(events))
	}
}

func TestUnmarshalEvent(t *testing.T) {
	var event Event

	s := `
	{
		"id": 1,
		"globalID": 1,
		"time": "2006-01-02T15:04:05.999999999Z",
		"type": "Starting",
		"data": {}
	}`

	if err := json.Unmarshal([]byte(s), &event); err != nil {
		t.Fatal("Failed to unmarshal event:", err)
	}
}

func TestUnsubscribeContention(t *testing.T) {
	// Check that we can unsubscribe without blocking the whole system.

	const (
		listeners = 50
		senders   = 1000
	)

	l, cancel := setupLogger()
	defer cancel()

	// Start listeners. These will poll until the stop channel is closed,
	// then exit and unsubscribe.

	stopListeners := make(chan struct{})
	var listenerWg sync.WaitGroup
	listenerWg.Add(listeners)
	for i := 0; i < listeners; i++ {
		go func() {
			defer listenerWg.Done()

			s := l.Subscribe(AllEvents)
			defer s.Unsubscribe()

			for {
				select {
				case <-s.C():

				case <-stopListeners:
					return
				}
			}
		}()
	}

	// Start senders. These send pointless events until the stop channel is
	// closed.

	stopSenders := make(chan struct{})
	defer close(stopSenders)
	var senderWg sync.WaitGroup
	senderWg.Add(senders)
	for i := 0; i < senders; i++ {
		go func() {
			defer senderWg.Done()

			t := time.NewTicker(time.Millisecond)

			for {
				select {
				case <-t.C:
					l.Log(StateChanged, nil)

				case <-stopSenders:
					return
				}
			}
		}()
	}

	// Give everything time to start up.

	time.Sleep(time.Second)

	// Stop the listeners and wait for them to exit. This should happen in a
	// reasonable time frame.

	t0 := time.Now()
	close(stopListeners)
	listenerWg.Wait()
	if d := time.Since(t0); d > time.Minute {
		t.Error("It should not take", d, "to unsubscribe from an event stream")
	}
}

func BenchmarkLogEvent(b *testing.B) {
	l, cancel := setupLogger()
	defer cancel()

	s := l.Subscribe(AllEvents)
	defer s.Unsubscribe()
	NewBufferedSubscription(s, 1) // runs in the background

	for i := 0; i < b.N; i++ {
		l.Log(StateChanged, nil)
	}
}
