// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"math/rand"

	// "github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/memsize"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

var (
	availableMemory int64
	totalLimit1     int64
	totalLimit2     int64
	totalLimitHard  int64
	limit1          int64
	limit2          int64
)

func init() {
	if total, err := memsize.MemorySize(); err != nil {
		// availableMemory = config.Options().FallBackMemoryLimit
		availableMemory = 100 << protocol.MiB
	} else {
		availableMemory = total
	}
	totalLimit1 = availableMemory / 100 * 45
	totalLimit2 = availableMemory / 100 * 50
	totalLimitHard = availableMemory / 100 * 55
	limit1 = availableMemory / 100 * 10
	limit2 = availableMemory / 100 * 1
}

// Tracks active disk overflow types, with information about how much memory
// they currently occupy and whether they were already told to spill to disk.
var lim = limiter{
	sizes:    make(map[int]int64),
	spilling: make(map[int]struct{}),
	mut:      sync.NewRWMutex(),
}

type limiter struct {
	totalSize int64
	sizes     map[int]int64
	spilling  map[int]struct{}
	mut       sync.RWMutex
}

func (li *limiter) register() int {
	var key int
	li.mut.Lock()
	for ok := true; ok; _, ok = li.sizes[key] {
		key = rand.Int()
	}
	li.sizes[key] = 0
	li.mut.Unlock()
	return key
}

func (li *limiter) deregister(key int) {
	li.mut.Lock()
	li.totalSize -= li.sizes[key]
	delete(li.sizes, key)
	delete(li.spilling, key)
	li.mut.Unlock()
}

// The returned bool is true, if the container should spill to disk
func (li *limiter) add(key int, size int64) bool {
	li.mut.Lock()
	defer li.mut.Unlock()
	if _, ok := li.spilling[key]; ok || li.shouldSpillLocked(key) {
		return true
	}
	li.totalSize += size
	li.sizes[key] += size
	return false
}

func (li *limiter) bytes(key int) int64 {
	li.mut.RLock()
	defer li.mut.RUnlock()
	return li.sizes[key]
}

func (li *limiter) remove(key int, size int64) {
	li.mut.Lock()
	li.totalSize -= size
	li.sizes[key] -= size
	li.mut.Unlock()
}

func (li *limiter) shouldSpillLocked(key int) bool {
	switch s := li.sizes[key]; {
	case li.totalSize > totalLimitHard:
		return true
	case li.totalSize > totalLimit2:
		return s > limit2
	case li.totalSize > totalLimit1:
		return s > limit1
	}
	return false
}
