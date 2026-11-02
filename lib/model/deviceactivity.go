// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"slices"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	assumedRate = 1 << 20       // 1 MiB/s
	minimumRate = 128 << 10 / 8 // 128 KiB/s
)

// deviceActivity tracks the number of outstanding requests per device and can
// answer which device is least busy. It is safe for use from multiple
// goroutines.
type deviceActivity struct {
	act  map[protocol.DeviceID]int             // device ID -> outstanding bytes
	rate map[protocol.DeviceID][]usageInterval // device ID -> usage, sorted by end time
	mut  sync.Mutex
}

type usageInterval struct {
	start time.Time
	end   time.Time
	rate  float64
}

func newDeviceActivity() *deviceActivity {
	return &deviceActivity{
		act:  make(map[protocol.DeviceID]int),
		rate: make(map[protocol.DeviceID][]usageInterval),
	}
}

// Returns the index of the least busy device, or -1 if there are no
// available devices.
func (m *deviceActivity) leastBusy(availability []Availability) int {
	best := -1
	var shortestQueue float64

	m.mut.Lock()
	for i, a := range availability {
		if wt := m.waitTimeLocked(a.ID); shortestQueue == 0 || wt < shortestQueue {
			shortestQueue = wt
			best = i
		}
	}
	m.mut.Unlock()

	return best
}

type token struct {
	id    protocol.DeviceID
	bytes int
	start time.Time
}

func (m *deviceActivity) using(id protocol.DeviceID, bytes int, start time.Time) token {
	m.mut.Lock()
	m.act[id] += bytes
	m.mut.Unlock()
	return token{id: id, bytes: bytes, start: start}
}

func (m *deviceActivity) done(tok token, end time.Time) {
	m.mut.Lock()
	m.act[tok.id] -= tok.bytes
	m.rate[tok.id] = appendRate(m.rate[tok.id], tok, end, 10)
	m.mut.Unlock()
}

func (m *deviceActivity) skip(tok token) {
	m.mut.Lock()
	m.act[tok.id] -= tok.bytes
	m.mut.Unlock()
}

// waitTimeLocked returns how long we'll need to wait for the currently
// queued requests for the given device to resolve.
func (m *deviceActivity) waitTimeLocked(id protocol.DeviceID) float64 {
	var rate float64 = assumedRate
	var secs float64
	if len(m.rate[id]) > 0 {
		sr := sliceRates(m.rate[id])
		for _, usage := range sr {
			dur := usage.end.Sub(usage.start).Seconds()
			rate += usage.rate * dur
			secs += dur
		}
		rate /= secs
		if rate < minimumRate {
			rate = minimumRate
		}
	}

	// Calculate how long the current request queue is, in seconds. We add a
	// constant overhead factor so that the queue time remains dependent on
	// the observed rate even when there are no currently outstanding
	// requests, and so that we never return a zero wait time.
	return float64(m.act[id]+protocol.MinBlockSize) / float64(rate)
}

func sliceRates(intvs []usageInterval) []usageInterval {
	farFuture := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	intvs = slices.Clone(intvs)
	slices.SortFunc(intvs, func(a, b usageInterval) int { return a.start.Compare(b.start) })

	t0 := intvs[0].start
	var rates []usageInterval

	var n int
	for {
		n++
		if n > 100 {
			break
		}

		t1 := farFuture
		for i := range intvs {
			intv := &intvs[i]
			if intv.rate == 0 {
				continue
			}
			if !intv.end.After(t0) {
				intv.rate = 0
				continue
			}
			if intv.start.After(t0) && intv.start.Before(t1) {
				t1 = intv.start
				continue
			}
			if intv.end.After(t0) && intv.end.Before(t1) {
				t1 = intv.end
			}
		}
		if t1.Equal(farFuture) {
			break
		}

		var rate float64
		for _, intv := range intvs {
			if intv.rate == 0 {
				continue
			}
			if !intv.start.After(t0) && !intv.end.Before(t1) {
				rate += intv.rate
			}
		}

		if rate > 0 {
			rates = append(rates, usageInterval{
				start: t0,
				end:   t1,
				rate:  rate,
			})
		}

		t0 = t1
	}

	return rates
}

func appendRate(l []usageInterval, tok token, end time.Time, maxL int) []usageInterval {
	intv := usageInterval{
		start: tok.start,
		end:   end,
		rate:  float64(tok.bytes) / end.Sub(tok.start).Seconds(),
	}
	if len(l) >= maxL {
		copy(l, l[1:])
		l[len(l)-1] = intv
	} else {
		l = append(l, intv)
	}
	return l
}
