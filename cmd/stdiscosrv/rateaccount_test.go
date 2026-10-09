// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestLimiterSlidingWindow(t *testing.T) {
	const window = 10 * time.Second
	start := time.Unix(1700000000, 0)

	// The current count contributes in full; the previous count decays
	// linearly as the current window elapses. Exceeding max rejects the event.
	cases := []struct {
		name    string
		cur     int32
		prev    int32
		offset  time.Duration
		max     int
		allowed bool
	}{
		{"current below limit at start", 8, 0, 0, 10, true},
		{"current at limit at start", 10, 0, 0, 10, false},
		{"current at limit halfway", 10, 0, window / 2, 10, false},
		{"previous counts at start", 0, 10, 0, 10, false},
		{"previous decays halfway", 3, 10, window / 2, 10, true},
		{"combined counts at limit", 5, 10, window / 2, 10, false},
		{"previous decays near end", 7, 10, 9 * time.Second, 10, true},
		{"fractional count below limit", 8, 5, 7 * time.Second, 10, true},
		{"fractional count above limit", 9, 5, 7 * time.Second, 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := limiter{starts: start.UnixNano(), cur: tc.cur, prev: tc.prev}
			if got := l.allow(start.Add(tc.offset), window, tc.max); got != tc.allowed {
				t.Errorf("allow() = %v, want %v", got, tc.allowed)
			}
			if l.cur != tc.cur+1 {
				t.Errorf("current count = %d, want %d (including rejected attempts)", l.cur, tc.cur+1)
			}
		})
	}
}

func TestAccountantRetryAfter(t *testing.T) {
	const window = time.Minute
	var deviceID protocol.DeviceID
	cases := []struct {
		name       string
		cur, prev  int32
		allowed    bool
		retryAfter time.Duration
		header     string
	}{
		{"within limit", 9, 0, true, 0, ""},
		{"just over limit", 10, 0, false, 66 * time.Second, "66"},
		{"double limit", 19, 0, false, 2 * window, "120"},
		{"triple limit", 29, 0, false, 3 * window, "180"},
		{"previous bucket rejects", 0, 20, false, window, "60"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := accountant{
				max: 10, window: window,
				rls: map[protocol.DeviceID]*limiter{
					deviceID: {starts: time.Now().UnixNano(), cur: tc.cur, prev: tc.prev},
				},
			}
			allowed, retryAfter := a.allow(&deviceID)
			if allowed != tc.allowed || retryAfter != tc.retryAfter {
				t.Fatalf("allow() = (%v, %s), want (%v, %s)", allowed, retryAfter, tc.allowed, tc.retryAfter)
			}
			if allowed {
				return
			}
			w := httptest.NewRecorder()
			a.reject(w, retryAfter)
			if w.Code != http.StatusTooManyRequests {
				t.Errorf("status = %d, want 429", w.Code)
			}
			if got := w.Header().Get("Retry-After"); got != tc.header {
				t.Errorf("Retry-After = %q, want %q", got, tc.header)
			}
			if got := w.Header().Get("x-Permitted-Requests"); got != "10 per 1m0s" {
				t.Errorf("x-Permitted-Requests = %q, want %q", got, "10 per 1m0s")
			}
		})
	}
}

func TestAccountantRetryAfterRoundsUp(t *testing.T) {
	a := accountant{max: 10, window: time.Second}
	w := httptest.NewRecorder()
	a.reject(w, 1100*time.Millisecond)
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, want 2", got)
	}
}

func TestLimiterWindowRollover(t *testing.T) {
	const window = 10 * time.Second
	start := time.Unix(1700000000, 0)
	cases := []struct {
		name   string
		offset time.Duration
		starts time.Duration
		cur    int32
		prev   int32
	}{
		{"before boundary", window - time.Nanosecond, 0, 5, 7},
		{"at boundary", window, window, 1, 4},
		{"after boundary", window + time.Nanosecond, window, 1, 4},
		{"before two windows", 2*window - time.Nanosecond, window, 1, 4},
		{"at two windows", 2 * window, 2 * window, 1, 0},
		{"after two windows", 2*window + time.Nanosecond, 2*window + time.Nanosecond, 1, 0},
		{"long idle", 10 * window, 10 * window, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := limiter{starts: start.UnixNano(), cur: 4, prev: 7}
			if !l.allow(start.Add(tc.offset), window, 100) {
				t.Error("event below limit was rejected")
			}
			want := limiter{starts: start.Add(tc.starts).UnixNano(), cur: tc.cur, prev: tc.prev}
			if l != want {
				t.Errorf("state = %+v, want %+v", l, want)
			}
		})
	}
}

func TestLimiterBurstAndRecovery(t *testing.T) {
	const window = 10 * time.Second
	start := time.Unix(1700000000, 0)
	var l limiter
	for i := 1; i <= 12; i++ {
		want := i <= 10
		if got := l.allow(start, window, 10); got != want {
			t.Errorf("burst event %d: allow() = %v, want %v", i, got, want)
		}
	}
	if l.cur != 12 {
		t.Errorf("current count = %d, want 12", l.cur)
	}
	if l.allow(start.Add(window+time.Nanosecond), window, 10) {
		t.Error("burst should still limit events just after rollover")
	}
	if !l.allow(start.Add(window+window/2), window, 10) {
		t.Error("event should be allowed after previous-window count decays")
	}
	if !l.allow(start.Add(4*window), window, 10) {
		t.Error("first event after idle should be allowed")
	}
	if want := (limiter{starts: start.Add(4 * window).UnixNano(), cur: 1}); l != want {
		t.Errorf("state after idle = %+v, want %+v", l, want)
	}
}
