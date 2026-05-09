// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// deviceActivity tracks the number of outstanding requests per device and can
// answer which device is least busy. It is safe for use from multiple
// goroutines.
type deviceActivity struct {
	act        map[protocol.DeviceID]int
	perf       map[protocol.DeviceID]devicePerformance
	roundRobin int
	mut        sync.Mutex
}

type devicePerformance struct {
	requests     int
	nanosPerByte float64
}

func newDeviceActivity() *deviceActivity {
	return &deviceActivity{
		act:  make(map[protocol.DeviceID]int),
		perf: make(map[protocol.DeviceID]devicePerformance),
	}
}

// Returns the index of the least busy device, or -1 if all are too busy.
func (m *deviceActivity) leastBusy(availability []Availability) int {
	m.mut.Lock()
	defer m.mut.Unlock()

	low := 2<<30 - 1
	best := -1
	bestPerf := devicePerformance{}
	bestPerfOK := false
	start := 0
	if len(availability) > 0 {
		start = m.roundRobin % len(availability)
		m.roundRobin++
	}
	for offset := range availability {
		i := (start + offset) % len(availability)
		usage := m.act[availability[i].ID]
		if usage < low {
			low = usage
			best = i
			bestPerf, bestPerfOK = m.perf[availability[i].ID]
			continue
		}
		if usage != low {
			continue
		}
		perf, perfOK := m.perf[availability[i].ID]
		if betterDevicePerformance(perf, perfOK, bestPerf, bestPerfOK) {
			best = i
			bestPerf = perf
			bestPerfOK = perfOK
		}
	}
	return best
}

func betterDevicePerformance(a devicePerformance, aOK bool, b devicePerformance, bOK bool) bool {
	if !aOK || !bOK {
		return false
	}
	return a.nanosPerByte < b.nanosPerByte
}

func (m *deviceActivity) using(availability Availability) {
	m.mut.Lock()
	m.act[availability.ID]++
	m.mut.Unlock()
}

func (m *deviceActivity) done(availability Availability) {
	m.mut.Lock()
	m.act[availability.ID]--
	m.mut.Unlock()
}

func (m *deviceActivity) success(availability Availability, duration time.Duration, bytes int) {
	if duration <= 0 || bytes <= 0 {
		return
	}

	m.mut.Lock()
	perf := m.perf[availability.ID]
	sample := float64(duration.Nanoseconds()) / float64(bytes)
	if perf.requests == 0 {
		perf.nanosPerByte = sample
	} else {
		perf.nanosPerByte = perf.nanosPerByte*0.8 + sample*0.2
	}
	perf.requests++
	m.perf[availability.ID] = perf
	m.mut.Unlock()
}
