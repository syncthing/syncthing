// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sync

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var timeNow = time.Now

type Mutex interface {
	Lock()
	Unlock()
}

type RWMutex interface {
	Mutex
	RLock()
	RUnlock()
}

type WaitGroup interface {
	Add(int)
	Done()
	Wait()
}

func NewMutex() Mutex {
	if debug {
		mutex := &loggedMutex{}
		mutex.holder.Store(holder{})
		return mutex
	}
	return &sync.Mutex{}
}

func NewRWMutex() RWMutex {
	if debug {
		mutex := &loggedRWMutex{
			readHolders: make(map[int][]holder),
			unlockers:   make(chan holder, 1024),
		}
		mutex.holder.Store(holder{})
		return mutex
	}
	return &sync.RWMutex{}
}

func NewWaitGroup() WaitGroup {
	if debug {
		return &loggedWaitGroup{}
	}
	return &sync.WaitGroup{}
}

type holder struct {
	at   string
	time time.Time
	goid int
}

func (h holder) String() string {
	if h.at == "" {
		return "not held"
	}
	return fmt.Sprintf("at %s goid: %d for %s", h.at, h.goid, timeNow().Sub(h.time))
}

type loggedMutex struct {
	sync.Mutex
	holder atomic.Value
}

func (m *loggedMutex) Lock() {
	m.Mutex.Lock()
	m.holder.Store(getHolder())
}

func (m *loggedMutex) Unlock() {
	currentHolder := m.holder.Load().(holder)
	duration := timeNow().Sub(currentHolder.time)
	if duration >= threshold {
		l.Debugf("Mutex held for %v. Locked at %s unlocked at %s", duration, currentHolder.at, getHolder().at)
	}
	m.holder.Store(holder{})
	m.Mutex.Unlock()
}

func (m *loggedMutex) Holders() string {
	return m.holder.Load().(holder).String()
}

type loggedRWMutex struct {
	sync.RWMutex
	holder atomic.Value

	readHolders    map[int][]holder
	readHoldersMut sync.Mutex

	logUnlockers atomic.Bool
	unlockers    chan holder
}

func (m *loggedRWMutex) Lock() {
	start := timeNow()

	m.logUnlockers.Store(true)
	m.RWMutex.Lock()
	m.logUnlockers.Store(false)

	holder := getHolder()
	m.holder.Store(holder)

	duration := holder.time.Sub(start)

	if duration > threshold {
		var unlockerStrings []string
	loop:
		for {
			select {
			case holder := <-m.unlockers:
				unlockerStrings = append(unlockerStrings, holder.String())
			default:
				break loop
			}
		}
		l.Debugf("RWMutex took %v to lock. Locked at %s. RUnlockers while locking:\n%s", duration, holder.at, strings.Join(unlockerStrings, "\n"))
	}
}

func (m *loggedRWMutex) Unlock() {
	currentHolder := m.holder.Load().(holder)
	duration := timeNow().Sub(currentHolder.time)
	if duration >= threshold {
		l.Debugf("RWMutex held for %v. Locked at %s unlocked at %s", duration, currentHolder.at, getHolder().at)
	}
	m.holder.Store(holder{})
	m.RWMutex.Unlock()
}

func (m *loggedRWMutex) RLock() {
	m.RWMutex.RLock()
	holder := getHolder()
	m.readHoldersMut.Lock()
	m.readHolders[holder.goid] = append(m.readHolders[holder.goid], holder)
	m.readHoldersMut.Unlock()
}

func (m *loggedRWMutex) RUnlock() {
	id := goid()
	m.readHoldersMut.Lock()
	current := m.readHolders[id]
	if len(current) > 0 {
		m.readHolders[id] = current[:len(current)-1]
	}
	m.readHoldersMut.Unlock()
	if m.logUnlockers.Load() {
		holder := getHolder()
		select {
		case m.unlockers <- holder:
		default:
			l.Debugf("Dropped holder %s as channel full", holder)
		}
	}
	m.RWMutex.RUnlock()
}

func (m *loggedRWMutex) Holders() string {
	output := m.holder.Load().(holder).String() + " (writer)"
	m.readHoldersMut.Lock()
	for _, holders := range m.readHolders {
		for _, holder := range holders {
			output += "\n" + holder.String() + " (reader)"
		}
	}
	m.readHoldersMut.Unlock()
	return output
}

type loggedWaitGroup struct {
	sync.WaitGroup
}

func (wg *loggedWaitGroup) Wait() {
	start := timeNow()
	wg.WaitGroup.Wait()
	duration := timeNow().Sub(start)
	if duration >= threshold {
		l.Debugf("WaitGroup took %v at %s", duration, getHolder())
	}
}

func getHolder() holder {
	_, file, line, _ := runtime.Caller(2)
	file = filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file))
	return holder{
		at:   fmt.Sprintf("%s:%d", file, line),
		goid: goid(),
		time: timeNow(),
	}
}

func goid() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, err := strconv.Atoi(idField)
	if err != nil {
		return -1
	}
	return id
}

// TimeoutCond is a variant on Cond. It has roughly the same semantics regarding 'L' - it must be held
// both when broadcasting and when calling TimeoutCondWaiter.Wait()
// Call Broadcast() to broadcast to all waiters on the TimeoutCond. Call SetupWait to create a
// TimeoutCondWaiter configured with the given timeout, which can then be used to listen for
// broadcasts.
type TimeoutCond struct {
	L  sync.Locker
	ch chan struct{}
}

// TimeoutCondWaiter is a type allowing a consumer to wait on a TimeoutCond with a timeout. Wait() may be called multiple times,
// and will return true every time that the TimeoutCond is broadcast to. Once the configured timeout
// expires, Wait() will return false.
// Call Stop() to release resources once this TimeoutCondWaiter is no longer needed.
type TimeoutCondWaiter struct {
	c     *TimeoutCond
	timer *time.Timer
}

func NewTimeoutCond(l sync.Locker) *TimeoutCond {
	return &TimeoutCond{
		L: l,
	}
}

func (c *TimeoutCond) Broadcast() {
	// ch.L must be locked when calling this function

	if c.ch != nil {
		close(c.ch)
		c.ch = nil
	}
}

func (c *TimeoutCond) SetupWait(timeout time.Duration) *TimeoutCondWaiter {
	timer := time.NewTimer(timeout)

	return &TimeoutCondWaiter{
		c:     c,
		timer: timer,
	}
}

func (w *TimeoutCondWaiter) Wait() bool {
	// ch.L must be locked when calling this function

	// Ensure that the channel exists, since we're going to be waiting on it
	if w.c.ch == nil {
		w.c.ch = make(chan struct{})
	}
	ch := w.c.ch

	w.c.L.Unlock()
	defer w.c.L.Lock()

	select {
	case <-w.timer.C:
		return false
	case <-ch:
		return true
	}
}

func (w *TimeoutCondWaiter) Stop() {
	w.timer.Stop()
}
