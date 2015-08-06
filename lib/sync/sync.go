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
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	if debug {
		return &loggedMutex{}
	}
	return &sync.Mutex{}
}

func NewRWMutex() RWMutex {
	if debug {
		return &loggedRWMutex{
			unlockers: make([]string, 0),
		}
	}
	return &sync.RWMutex{}
}

func NewWaitGroup() WaitGroup {
	if debug {
		return &loggedWaitGroup{}
	}
	return &sync.WaitGroup{}
}

type loggedMutex struct {
	sync.Mutex
	start    time.Time
	lockedAt string
}

func (m *loggedMutex) Lock() {
	m.Mutex.Lock()
	m.start = time.Now()
	m.lockedAt = getCaller()
}

func (m *loggedMutex) Unlock() {
	duration := time.Now().Sub(m.start)
	if duration >= threshold {
		l.Debugf("Mutex held for %v. Locked at %s unlocked at %s", duration, m.lockedAt, getCaller())
	}
	m.Mutex.Unlock()
}

type loggedRWMutex struct {
	sync.RWMutex
	start    time.Time
	lockedAt string

	logUnlockers uint32

	unlockers    []string
	unlockersMut sync.Mutex
}

func (m *loggedRWMutex) Lock() {
	start := time.Now()

	atomic.StoreUint32(&m.logUnlockers, 1)
	m.RWMutex.Lock()
	m.logUnlockers = 0

	m.start = time.Now()
	duration := m.start.Sub(start)

	m.lockedAt = getCaller()
	if duration > threshold {
		l.Debugf("RWMutex took %v to lock. Locked at %s. RUnlockers while locking: %s", duration, m.lockedAt, strings.Join(m.unlockers, ", "))
	}
	m.unlockers = m.unlockers[0:]
}

func (m *loggedRWMutex) Unlock() {
	duration := time.Now().Sub(m.start)
	if duration >= threshold {
		l.Debugf("RWMutex held for %v. Locked at %s: unlocked at %s", duration, m.lockedAt, getCaller())
	}
	m.RWMutex.Unlock()
}

func (m *loggedRWMutex) RUnlock() {
	if atomic.LoadUint32(&m.logUnlockers) == 1 {
		m.unlockersMut.Lock()
		m.unlockers = append(m.unlockers, getCaller())
		m.unlockersMut.Unlock()
	}
	m.RWMutex.RUnlock()
}

type loggedWaitGroup struct {
	sync.WaitGroup
}

func (wg *loggedWaitGroup) Wait() {
	start := time.Now()
	wg.WaitGroup.Wait()
	duration := time.Now().Sub(start)
	if duration >= threshold {
		l.Debugf("WaitGroup took %v at %s", duration, getCaller())
	}
}

func getCaller() string {
	_, file, line, _ := runtime.Caller(2)
	file = filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file))
	return fmt.Sprintf("%s:%d", file, line)
}
