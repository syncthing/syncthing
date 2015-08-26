// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package events provides event subscription and polling functionality.
package events

import (
	"errors"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/sync"
)

type EventType int

const (
	Ping EventType = 1 << iota
	Starting
	StartupComplete
	DeviceDiscovered
	DeviceConnected
	DeviceDisconnected
	DeviceRejected
	DevicePaused
	DeviceResumed
	LocalIndexUpdated
	RemoteIndexUpdated
	ItemStarted
	ItemFinished
	StateChanged
	FolderRejected
	ConfigSaved
	DownloadProgress
	FolderSummary
	FolderCompletion
	FolderErrors
	FolderScanProgress

	AllEvents = (1 << iota) - 1
)

func (t EventType) String() string {
	switch t {
	case Ping:
		return "Ping"
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
	case ConfigSaved:
		return "ConfigSaved"
	case DownloadProgress:
		return "DownloadProgress"
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
	case FolderScanProgress:
		return "FolderScanProgress"
	default:
		return "Unknown"
	}
}

func (t EventType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

const BufferSize = 64

type Logger struct {
	subs   map[int]*Subscription
	nextID int
	mutex  sync.Mutex
}

type Event struct {
	ID   int         `json:"id"`
	Time time.Time   `json:"time"`
	Type EventType   `json:"type"`
	Data interface{} `json:"data"`
}

type Subscription struct {
	mask    EventType
	id      int
	events  chan Event
	timeout *time.Timer
}

var Default = NewLogger()

var (
	ErrTimeout = errors.New("timeout")
	ErrClosed  = errors.New("closed")
)

func NewLogger() *Logger {
	return &Logger{
		subs:  make(map[int]*Subscription),
		mutex: sync.NewMutex(),
	}
}

func (l *Logger) Log(t EventType, data interface{}) {
	l.mutex.Lock()
	if debug {
		dl.Debugln("log", l.nextID, t.String(), data)
	}
	e := Event{
		ID:   l.nextID,
		Time: time.Now(),
		Type: t,
		Data: data,
	}
	l.nextID++
	for _, s := range l.subs {
		if s.mask&t != 0 {
			select {
			case s.events <- e:
			default:
				// if s.events is not ready, drop the event
			}
		}
	}
	l.mutex.Unlock()
}

func (l *Logger) Subscribe(mask EventType) *Subscription {
	l.mutex.Lock()
	if debug {
		dl.Debugln("subscribe", mask)
	}
	s := &Subscription{
		mask:    mask,
		id:      l.nextID,
		events:  make(chan Event, BufferSize),
		timeout: time.NewTimer(0),
	}
	l.nextID++
	l.subs[s.id] = s
	l.mutex.Unlock()
	return s
}

func (l *Logger) Unsubscribe(s *Subscription) {
	l.mutex.Lock()
	if debug {
		dl.Debugln("unsubscribe")
	}
	delete(l.subs, s.id)
	close(s.events)
	l.mutex.Unlock()
}

// Poll returns an event from the subscription or an error if the poll times
// out of the event channel is closed. Poll should not be called concurrently
// from multiple goroutines for a single subscription.
func (s *Subscription) Poll(timeout time.Duration) (Event, error) {
	if debug {
		dl.Debugln("poll", timeout)
	}

	if !s.timeout.Reset(timeout) {
		select {
		case <-s.timeout.C:
		default:
		}
	}

	select {
	case e, ok := <-s.events:
		if !ok {
			return e, ErrClosed
		}
		s.timeout.Stop()
		return e, nil
	case <-s.timeout.C:
		return Event{}, ErrTimeout
	}
}

func (s *Subscription) C() <-chan Event {
	return s.events
}

type BufferedSubscription struct {
	sub  *Subscription
	buf  []Event
	next int
	cur  int
	mut  sync.Mutex
	cond *stdsync.Cond
}

func NewBufferedSubscription(s *Subscription, size int) *BufferedSubscription {
	bs := &BufferedSubscription{
		sub: s,
		buf: make([]Event, size),
		mut: sync.NewMutex(),
	}
	bs.cond = stdsync.NewCond(bs.mut)
	go bs.pollingLoop()
	return bs
}

func (s *BufferedSubscription) pollingLoop() {
	for {
		ev, err := s.sub.Poll(60 * time.Second)
		if err == ErrTimeout {
			continue
		}
		if err == ErrClosed {
			return
		}
		if err != nil {
			panic("unexpected error: " + err.Error())
		}

		s.mut.Lock()
		s.buf[s.next] = ev
		s.next = (s.next + 1) % len(s.buf)
		s.cur = ev.ID
		s.cond.Broadcast()
		s.mut.Unlock()
	}
}

func (s *BufferedSubscription) Since(id int, into []Event) []Event {
	s.mut.Lock()
	defer s.mut.Unlock()

	for id >= s.cur {
		s.cond.Wait()
	}

	for i := s.next; i < len(s.buf); i++ {
		if s.buf[i].ID > id {
			into = append(into, s.buf[i])
		}
	}
	for i := 0; i < s.next; i++ {
		if s.buf[i].ID > id {
			into = append(into, s.buf[i])
		}
	}

	return into
}

// Error returns a string pointer suitable for JSON marshalling errors. It
// retains the "null on sucess" semantics, but ensures the error result is a
// string regardless of the underlying concrete error type.
func Error(err error) *string {
	if err == nil {
		return nil
	}
	str := err.Error()
	return &str
}
