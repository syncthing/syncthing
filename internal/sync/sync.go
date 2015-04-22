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
	"sync"
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
		return &loggedRWMutex{}
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
}

func (m *loggedRWMutex) Lock() {
	start := time.Now()

	m.RWMutex.Lock()

	m.start = time.Now()
	duration := m.start.Sub(start)

	m.lockedAt = getCaller()
	if duration > threshold {
		l.Debugf("RWMutex took %v to lock. Locked at %s", duration, m.lockedAt)
	}
}

func (m *loggedRWMutex) Unlock() {
	duration := time.Now().Sub(m.start)
	if duration >= threshold {
		l.Debugf("RWMutex held for %v. Locked at %s: unlocked at %s", duration, m.lockedAt, getCaller())
	}
	m.RWMutex.Unlock()
}

type loggedWaitGroup struct {
	sync.WaitGroup
}

func (wg *loggedWaitGroup) Done() {
	start := time.Now()
	wg.WaitGroup.Done()
	duration := time.Now().Sub(start)
	if duration > threshold {
		l.Debugf("WaitGroup took %v at %s", duration, getCaller())
	}
}

func getCaller() string {
	pc := make([]uintptr, 10)
	runtime.Callers(3, pc)
	f := runtime.FuncForPC(pc[0])
	file, line := f.FileLine(pc[0])
	file = filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file))
	return fmt.Sprintf("%s:%d", file, line)
}
