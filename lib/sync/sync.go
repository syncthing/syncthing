// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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

	"github.com/sasha-s/go-deadlock"
)

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
	if useDeadlock {
		return &deadlock.Mutex{}
	}
	if debug {
		mutex := &loggedMutex{}
		mutex.holder.Store(holder{})
		return mutex
	}
	return &sync.Mutex{}
}

func NewRWMutex() RWMutex {
	if useDeadlock {
		return &deadlock.RWMutex{}
	}
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
	return fmt.Sprintf("at %s goid: %d for %s", h.at, h.goid, time.Since(h.time))
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
	duration := time.Since(currentHolder.time)
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

	logUnlockers int32
	unlockers    chan holder
}

func (m *loggedRWMutex) Lock() {
	start := time.Now()

	atomic.StoreInt32(&m.logUnlockers, 1)
	m.RWMutex.Lock()
	m.logUnlockers = 0

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
	duration := time.Since(currentHolder.time)
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
	if atomic.LoadInt32(&m.logUnlockers) == 1 {
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
	start := time.Now()
	wg.WaitGroup.Wait()
	duration := time.Since(start)
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
		time: time.Now(),
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
