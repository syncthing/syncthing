// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package sync

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/calmh/logger"
)

func TestTypes(t *testing.T) {
	if _, ok := NewMutex().(*sync.Mutex); !ok {
		t.Error("Wrong type")
	}

	if _, ok := NewRWMutex().(*sync.RWMutex); !ok {
		t.Error("Wrong type")
	}

	if _, ok := NewWaitGroup().(*sync.WaitGroup); !ok {
		t.Error("Wrong type")
	}

	debug = true

	if _, ok := NewMutex().(*loggedMutex); !ok {
		t.Error("Wrong type")
	}

	if _, ok := NewRWMutex().(*loggedRWMutex); !ok {
		t.Error("Wrong type")
	}

	if _, ok := NewWaitGroup().(*loggedWaitGroup); !ok {
		t.Error("Wrong type")
	}

	debug = false
}

func TestMutex(t *testing.T) {
	debug = true
	threshold = time.Millisecond * 5

	msgmut := sync.Mutex{}
	messages := make([]string, 0)

	l.AddHandler(logger.LevelDebug, func(_ logger.LogLevel, message string) {
		msgmut.Lock()
		messages = append(messages, message)
		msgmut.Unlock()
	})

	mut := NewMutex()
	mut.Lock()
	time.Sleep(2 * time.Millisecond)
	mut.Unlock()

	if len(messages) > 0 {
		t.Errorf("Unexpected message count")
	}

	mut.Lock()
	time.Sleep(6 * time.Millisecond)
	mut.Unlock()

	if len(messages) != 1 {
		t.Errorf("Unexpected message count")
	}

	debug = false
}

func TestRWMutex(t *testing.T) {
	debug = true
	threshold = time.Millisecond * 5

	msgmut := sync.Mutex{}
	messages := make([]string, 0)

	l.AddHandler(logger.LevelDebug, func(_ logger.LogLevel, message string) {
		msgmut.Lock()
		messages = append(messages, message)
		msgmut.Unlock()
	})

	mut := NewRWMutex()
	mut.Lock()
	time.Sleep(2 * time.Millisecond)
	mut.Unlock()

	if len(messages) > 0 {
		t.Errorf("Unexpected message count")
	}

	mut.Lock()
	time.Sleep(6 * time.Millisecond)
	mut.Unlock()

	if len(messages) != 1 {
		t.Errorf("Unexpected message count")
	}

	// Testing rlocker logging
	mut.RLock()
	go func() {
		time.Sleep(7 * time.Millisecond)
		mut.RUnlock()
	}()

	mut.Lock()
	mut.Unlock()

	if len(messages) != 2 {
		t.Errorf("Unexpected message count")
	}
	if !strings.Contains(messages[1], "RUnlockers while locking: sync") || !strings.Contains(messages[1], "sync_test.go:") {
		t.Error("Unexpected message")
	}

	// Testing multiple rlockers
	mut.RLock()
	mut.RLock()
	mut.RLock()
	mut.RUnlock()
	mut.RUnlock()
	mut.RUnlock()

	debug = false
}

func TestWaitGroup(t *testing.T) {
	debug = true
	threshold = time.Millisecond * 5

	msgmut := sync.Mutex{}
	messages := make([]string, 0)

	l.AddHandler(logger.LevelDebug, func(_ logger.LogLevel, message string) {
		msgmut.Lock()
		messages = append(messages, message)
		msgmut.Unlock()
	})

	wg := NewWaitGroup()
	wg.Add(1)
	go func() {
		time.Sleep(2 * time.Millisecond)
		wg.Done()
	}()
	wg.Wait()

	if len(messages) > 0 {
		t.Errorf("Unexpected message count")
	}

	wg = NewWaitGroup()
	wg.Add(1)
	go func() {
		time.Sleep(6 * time.Millisecond)
		wg.Done()
	}()
	wg.Wait()

	if len(messages) != 1 {
		t.Errorf("Unexpected message count")
	}

	debug = false
}
