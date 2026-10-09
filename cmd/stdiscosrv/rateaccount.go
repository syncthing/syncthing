// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type accountant struct {
	name   string
	max    int
	window time.Duration
	mut    sync.Mutex
	rls    map[protocol.DeviceID]*limiter
}

func (s *accountant) reject(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	w.Header().Set("x-Permitted-Requests", fmt.Sprintf("%d per %s", s.max, s.window))
	http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
}

func (s *accountant) Serve(ctx context.Context) error {
	for {
		select {
		case <-time.After(time.Hour):
			// Once an hour we declare a general amnesty (and avoid
			// unbounded memory growth)
			s.mut.Lock()
			clear(s.rls)
			s.mut.Unlock()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// limiter implements a simple sliding-window rate limiter
type limiter struct {
	starts int64 // unix nanos
	cur    int32
	prev   int32
}

func (l *limiter) allow(when time.Time, window time.Duration, max int) bool {
	now := when.UnixNano()
	switch {
	case now-l.starts >= 2*window.Nanoseconds():
		// state is entirely cleared
		l.starts = now
		l.prev = 0
		l.cur = 1
		return true

	case now-l.starts >= window.Nanoseconds():
		// window has advanced
		l.starts += window.Nanoseconds()
		l.prev = l.cur
		l.cur = 1

	default:
		// we're in the current window
		l.cur++
	}

	// Determine if the event should have been allowed
	f := float64(now-l.starts) / float64(window.Nanoseconds())
	events := int(float64(l.cur) + (1-f)*float64(l.prev))
	return events <= max
}

func (a *accountant) allow(d *protocol.DeviceID) (bool, time.Duration) {
	a.mut.Lock()
	defer a.mut.Unlock()

	lim, ok := a.rls[*d]
	if !ok {
		lim = &limiter{}
		a.rls[*d] = lim
	}
	if lim.allow(time.Now(), a.window, a.max) {
		return true, 0
	}

	retryAfter := a.window * time.Duration(lim.cur) / time.Duration(a.max)
	return false, min(max(a.window, retryAfter), time.Hour)
}
