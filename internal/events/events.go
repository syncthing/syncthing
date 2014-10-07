// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

// Package events provides event subscription and polling functionality.
package events

import (
	"errors"
	"sync"
	"time"
)

type EventType uint64

const (
	Ping EventType = 1 << iota
	Starting
	StartupComplete
	DeviceDiscovered
	DeviceConnected
	DeviceDisconnected
	DeviceRejected
	LocalIndexUpdated
	RemoteIndexUpdated
	ItemStarted
	StateChanged
	FolderRejected
	ConfigSaved

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
	case StateChanged:
		return "StateChanged"
	case FolderRejected:
		return "FolderRejected"
	case ConfigSaved:
		return "ConfigSaved"
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
	nextId int
	mutex  sync.Mutex
}

type Event struct {
	ID   int         `json:"id"`
	Time time.Time   `json:"time"`
	Type EventType   `json:"type"`
	Data interface{} `json:"data"`
}

type Subscription struct {
	mask   EventType
	id     int
	events chan Event
	mutex  sync.Mutex
}

var Default = NewLogger()

var (
	ErrTimeout = errors.New("timeout")
	ErrClosed  = errors.New("closed")
)

func NewLogger() *Logger {
	return &Logger{
		subs: make(map[int]*Subscription),
	}
}

func (l *Logger) Log(t EventType, data interface{}) {
	l.mutex.Lock()
	if debug {
		dl.Debugln("log", l.nextId, t.String(), data)
	}
	e := Event{
		ID:   l.nextId,
		Time: time.Now(),
		Type: t,
		Data: data,
	}
	l.nextId++
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
		mask:   mask,
		id:     l.nextId,
		events: make(chan Event, BufferSize),
	}
	l.nextId++
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

func (s *Subscription) Poll(timeout time.Duration) (Event, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if debug {
		dl.Debugln("poll", timeout)
	}

	to := time.After(timeout)
	select {
	case e, ok := <-s.events:
		if !ok {
			return e, ErrClosed
		}
		return e, nil
	case <-to:
		return Event{}, ErrTimeout
	}
}

type BufferedSubscription struct {
	sub  *Subscription
	buf  []Event
	next int
	cur  int
	mut  sync.Mutex
	cond *sync.Cond
}

func NewBufferedSubscription(s *Subscription, size int) *BufferedSubscription {
	bs := &BufferedSubscription{
		sub: s,
		buf: make([]Event, size),
	}
	bs.cond = sync.NewCond(&bs.mut)
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
