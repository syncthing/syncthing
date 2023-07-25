// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate -command counterfeiter go run github.com/maxbrunsfeld/counterfeiter/v6
//go:generate counterfeiter -o mocks/buffered_subscription.go --fake-name BufferedSubscription . BufferedSubscription

// Package events provides event subscription and polling functionality.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/sync"
)

type EventType int64

const (
	Starting EventType = 1 << iota
	StartupComplete
	DeviceDiscovered
	DeviceConnected
	DeviceDisconnected
	DeviceRejected // DEPRECATED, superseded by PendingDevicesChanged
	PendingDevicesChanged
	DevicePaused
	DeviceResumed
	ClusterConfigReceived
	LocalChangeDetected
	RemoteChangeDetected
	LocalIndexUpdated
	RemoteIndexUpdated
	ItemStarted
	ItemFinished
	StateChanged
	FolderRejected // DEPRECATED, superseded by PendingFoldersChanged
	PendingFoldersChanged
	ConfigSaved
	DownloadProgress
	RemoteDownloadProgress
	FolderSummary
	FolderCompletion
	FolderErrors
	FolderScanProgress
	FolderPaused
	FolderResumed
	FolderWatchStateChanged
	ListenAddressesChanged
	LoginAttempt
	Failure

	AllEvents = (1 << iota) - 1
)

var (
	runningTests = false
	errNoop      = errors.New("method of a noop object called")
)

const eventLogTimeout = 15 * time.Millisecond

func (t EventType) String() string {
	switch t {
	case Starting:
		return "Starting"
	case StartupComplete:
		return "StartupComplete"
	case DeviceDiscovered:
		return "DeviceDiscovered"
	case DeviceConnected:
		return "DeviceConnected"
	case DeviceDisconnected:
		return "DeviceDisconnected"
	case DeviceRejected:
		return "DeviceRejected"
	case PendingDevicesChanged:
		return "PendingDevicesChanged"
	case LocalChangeDetected:
		return "LocalChangeDetected"
	case RemoteChangeDetected:
		return "RemoteChangeDetected"
	case LocalIndexUpdated:
		return "LocalIndexUpdated"
	case RemoteIndexUpdated:
		return "RemoteIndexUpdated"
	case ItemStarted:
		return "ItemStarted"
	case ItemFinished:
		return "ItemFinished"
	case StateChanged:
		return "StateChanged"
	case FolderRejected:
		return "FolderRejected"
	case PendingFoldersChanged:
		return "PendingFoldersChanged"
	case ConfigSaved:
		return "ConfigSaved"
	case DownloadProgress:
		return "DownloadProgress"
	case RemoteDownloadProgress:
		return "RemoteDownloadProgress"
	case FolderSummary:
		return "FolderSummary"
	case FolderCompletion:
		return "FolderCompletion"
	case FolderErrors:
		return "FolderErrors"
	case DevicePaused:
		return "DevicePaused"
	case DeviceResumed:
		return "DeviceResumed"
	case ClusterConfigReceived:
		return "ClusterConfigReceived"
	case FolderScanProgress:
		return "FolderScanProgress"
	case FolderPaused:
		return "FolderPaused"
	case FolderResumed:
		return "FolderResumed"
	case ListenAddressesChanged:
		return "ListenAddressesChanged"
	case LoginAttempt:
		return "LoginAttempt"
	case FolderWatchStateChanged:
		return "FolderWatchStateChanged"
	case Failure:
		return "Failure"
	default:
		return "Unknown"
	}
}

func (t EventType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *EventType) UnmarshalJSON(b []byte) error {
	var s string

	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	*t = UnmarshalEventType(s)

	return nil
}

func UnmarshalEventType(s string) EventType {
	switch s {
	case "Starting":
		return Starting
	case "StartupComplete":
		return StartupComplete
	case "DeviceDiscovered":
		return DeviceDiscovered
	case "DeviceConnected":
		return DeviceConnected
	case "DeviceDisconnected":
		return DeviceDisconnected
	case "DeviceRejected":
		return DeviceRejected
	case "PendingDevicesChanged":
		return PendingDevicesChanged
	case "LocalChangeDetected":
		return LocalChangeDetected
	case "RemoteChangeDetected":
		return RemoteChangeDetected
	case "LocalIndexUpdated":
		return LocalIndexUpdated
	case "RemoteIndexUpdated":
		return RemoteIndexUpdated
	case "ItemStarted":
		return ItemStarted
	case "ItemFinished":
		return ItemFinished
	case "StateChanged":
		return StateChanged
	case "FolderRejected":
		return FolderRejected
	case "PendingFoldersChanged":
		return PendingFoldersChanged
	case "ConfigSaved":
		return ConfigSaved
	case "DownloadProgress":
		return DownloadProgress
	case "RemoteDownloadProgress":
		return RemoteDownloadProgress
	case "FolderSummary":
		return FolderSummary
	case "FolderCompletion":
		return FolderCompletion
	case "FolderErrors":
		return FolderErrors
	case "DevicePaused":
		return DevicePaused
	case "DeviceResumed":
		return DeviceResumed
	case "ClusterConfigReceived":
		return ClusterConfigReceived
	case "FolderScanProgress":
		return FolderScanProgress
	case "FolderPaused":
		return FolderPaused
	case "FolderResumed":
		return FolderResumed
	case "ListenAddressesChanged":
		return ListenAddressesChanged
	case "LoginAttempt":
		return LoginAttempt
	case "FolderWatchStateChanged":
		return FolderWatchStateChanged
	case "Failure":
		return Failure
	default:
		return 0
	}
}

const BufferSize = 64

type Logger interface {
	suture.Service
	Log(t EventType, data interface{})
	Subscribe(mask EventType) Subscription
}

type logger struct {
	subs                []*subscription
	nextSubscriptionIDs []int
	nextGlobalID        int
	timeout             *time.Timer
	events              chan Event
	funcs               chan func(context.Context)
	toUnsubscribe       chan *subscription
}

type Event struct {
	// Per-subscription sequential event ID. Named "id" for backwards compatibility with the REST API
	SubscriptionID int `json:"id"`
	// Global ID of the event across all subscriptions
	GlobalID int         `json:"globalID"`
	Time     time.Time   `json:"time"`
	Type     EventType   `json:"type"`
	Data     interface{} `json:"data"`
}

type Subscription interface {
	C() <-chan Event
	Poll(timeout time.Duration) (Event, error)
	Mask() EventType
	Unsubscribe()
}

type subscription struct {
	mask          EventType
	events        chan Event
	toUnsubscribe chan *subscription
	timeout       *time.Timer
	ctx           context.Context
}

var (
	ErrTimeout = errors.New("timeout")
	ErrClosed  = errors.New("closed")
)

func NewLogger() Logger {
	l := &logger{
		timeout:       time.NewTimer(time.Second),
		events:        make(chan Event, BufferSize),
		funcs:         make(chan func(context.Context)),
		toUnsubscribe: make(chan *subscription),
	}
	// Make sure the timer is in the stopped state and hasn't fired anything
	// into the channel.
	if !l.timeout.Stop() {
		<-l.timeout.C
	}
	return l
}

func (l *logger) Serve(ctx context.Context) error {
loop:
	for {
		select {
		case e := <-l.events:
			// Incoming events get sent
			l.sendEvent(e)
			metricEvents.WithLabelValues(e.Type.String(), metricEventStateCreated).Inc()

		case fn := <-l.funcs:
			// Subscriptions are handled here.
			fn(ctx)

		case s := <-l.toUnsubscribe:
			l.unsubscribe(s)

		case <-ctx.Done():
			break loop
		}
	}

	// Closing the event channels corresponds to what happens when a
	// subscription is unsubscribed; this stops any BufferedSubscription,
	// makes Poll() return ErrClosed, etc.
	for _, s := range l.subs {
		close(s.events)
	}

	return nil
}

func (l *logger) Log(t EventType, data interface{}) {
	l.events <- Event{
		Time: time.Now(), // intentionally high precision
		Type: t,
		Data: data,
		// SubscriptionID and GlobalID are set in sendEvent
	}
}

func (l *logger) sendEvent(e Event) {
	l.nextGlobalID++
	dl.Debugln("log", l.nextGlobalID, e.Type, e.Data)

	e.GlobalID = l.nextGlobalID

	for i, s := range l.subs {
		if s.mask&e.Type != 0 {
			e.SubscriptionID = l.nextSubscriptionIDs[i]
			l.nextSubscriptionIDs[i]++

			l.timeout.Reset(eventLogTimeout)
			timedOut := false

			select {
			case s.events <- e:
				metricEvents.WithLabelValues(e.Type.String(), metricEventStateDelivered).Inc()
			case <-l.timeout.C:
				// if s.events is not ready, drop the event
				timedOut = true
				metricEvents.WithLabelValues(e.Type.String(), metricEventStateDropped).Inc()
			}

			// If stop returns false it already sent something to the
			// channel. If we didn't already read it above we must do so now
			// or we get a spurious timeout on the next loop.
			if !l.timeout.Stop() && !timedOut {
				<-l.timeout.C
			}
		}
	}
}

func (l *logger) Subscribe(mask EventType) Subscription {
	res := make(chan Subscription)
	l.funcs <- func(ctx context.Context) {
		dl.Debugln("subscribe", mask)

		s := &subscription{
			mask:          mask,
			events:        make(chan Event, BufferSize),
			toUnsubscribe: l.toUnsubscribe,
			timeout:       time.NewTimer(0),
			ctx:           ctx,
		}

		// We need to create the timeout timer in the stopped, non-fired state so
		// that Subscription.Poll() can safely reset it and select on the timeout
		// channel. This ensures the timer is stopped and the channel drained.
		if runningTests {
			// Make the behavior stable when running tests to avoid randomly
			// varying test coverage. This ensures, in practice if not in
			// theory, that the timer fires and we take the true branch of the
			// next if.
			runtime.Gosched()
		}
		if !s.timeout.Stop() {
			<-s.timeout.C
		}

		l.subs = append(l.subs, s)
		l.nextSubscriptionIDs = append(l.nextSubscriptionIDs, 1)
		res <- s
	}
	return <-res
}

func (l *logger) unsubscribe(s *subscription) {
	dl.Debugln("unsubscribe", s.mask)
	for i, ss := range l.subs {
		if s == ss {
			last := len(l.subs) - 1

			l.subs[i] = l.subs[last]
			l.subs[last] = nil
			l.subs = l.subs[:last]

			l.nextSubscriptionIDs[i] = l.nextSubscriptionIDs[last]
			l.nextSubscriptionIDs[last] = 0
			l.nextSubscriptionIDs = l.nextSubscriptionIDs[:last]

			break
		}
	}
	close(s.events)
}

func (l *logger) String() string {
	return fmt.Sprintf("events.Logger/@%p", l)
}

// Poll returns an event from the subscription or an error if the poll times
// out of the event channel is closed. Poll should not be called concurrently
// from multiple goroutines for a single subscription.
func (s *subscription) Poll(timeout time.Duration) (Event, error) {
	dl.Debugln("poll", timeout)

	s.timeout.Reset(timeout)

	select {
	case e, ok := <-s.events:
		if !ok {
			return e, ErrClosed
		}
		if runningTests {
			// Make the behavior stable when running tests to avoid randomly
			// varying test coverage. This ensures, in practice if not in
			// theory, that the timer fires and we take the true branch of
			// the next if.
			s.timeout.Reset(0)
			runtime.Gosched()
		}
		if !s.timeout.Stop() {
			// The timeout must be stopped and possibly drained to be ready
			// for reuse in the next call.
			<-s.timeout.C
		}
		return e, nil
	case <-s.timeout.C:
		return Event{}, ErrTimeout
	}
}

func (s *subscription) C() <-chan Event {
	return s.events
}

func (s *subscription) Mask() EventType {
	return s.mask
}

func (s *subscription) Unsubscribe() {
	select {
	case s.toUnsubscribe <- s:
	case <-s.ctx.Done():
	}
}

type bufferedSubscription struct {
	sub  Subscription
	buf  []Event
	next int
	cur  int // Current SubscriptionID
	mut  sync.Mutex
	cond *sync.TimeoutCond
}

type BufferedSubscription interface {
	Since(id int, into []Event, timeout time.Duration) []Event
	Mask() EventType
}

func NewBufferedSubscription(s Subscription, size int) BufferedSubscription {
	bs := &bufferedSubscription{
		sub: s,
		buf: make([]Event, size),
		mut: sync.NewMutex(),
	}
	bs.cond = sync.NewTimeoutCond(bs.mut)
	go bs.pollingLoop()
	return bs
}

func (s *bufferedSubscription) pollingLoop() {
	for ev := range s.sub.C() {
		s.mut.Lock()
		s.buf[s.next] = ev
		s.next = (s.next + 1) % len(s.buf)
		s.cur = ev.SubscriptionID
		s.cond.Broadcast()
		s.mut.Unlock()
	}
}

func (s *bufferedSubscription) Since(id int, into []Event, timeout time.Duration) []Event {
	s.mut.Lock()
	defer s.mut.Unlock()

	// Check once first before generating the TimeoutCondWaiter
	if id >= s.cur {
		waiter := s.cond.SetupWait(timeout)
		defer waiter.Stop()

		for id >= s.cur {
			if eventsAvailable := waiter.Wait(); !eventsAvailable {
				// Timed out
				return into
			}
		}
	}

	for i := s.next; i < len(s.buf); i++ {
		if s.buf[i].SubscriptionID > id {
			into = append(into, s.buf[i])
		}
	}
	for i := 0; i < s.next; i++ {
		if s.buf[i].SubscriptionID > id {
			into = append(into, s.buf[i])
		}
	}

	return into
}

func (s *bufferedSubscription) Mask() EventType {
	return s.sub.Mask()
}

// Error returns a string pointer suitable for JSON marshalling errors. It
// retains the "null on success" semantics, but ensures the error result is a
// string regardless of the underlying concrete error type.
func Error(err error) *string {
	if err == nil {
		return nil
	}
	str := err.Error()
	return &str
}

type noopLogger struct{}

var NoopLogger Logger = &noopLogger{}

func (*noopLogger) Serve(_ context.Context) error { return nil }

func (*noopLogger) Log(_ EventType, _ interface{}) {}

func (*noopLogger) Subscribe(_ EventType) Subscription {
	return &noopSubscription{}
}

type noopSubscription struct{}

func (*noopSubscription) C() <-chan Event {
	return nil
}

func (*noopSubscription) Poll(_ time.Duration) (Event, error) {
	return Event{}, errNoop
}

func (*noopSubscription) Mask() EventType {
	return 0
}

func (*noopSubscription) Unsubscribe() {}
