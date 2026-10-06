// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/syncthing/syncthing/lib/protocol"
)

type deviceLimiters struct {
	name     string
	max      int
	window   time.Duration
	mut      sync.Mutex
	limiters map[protocol.DeviceID]*limiter
	metric   prometheus.Histogram
}

func newDeviceLimiters(name string, max int, window time.Duration) *deviceLimiters {
	buckets := []float64{0}
	prev := 0.0
	mul := 1.0 / 8
	for mul <= 8 {
		v := math.Ceil(float64(max) * mul)
		if v > prev {
			buckets = append(buckets, v)
			prev = v
		}
		mul *= 2
	}
	return &deviceLimiters{
		name:     name,
		max:      max,
		window:   window,
		limiters: make(map[protocol.DeviceID]*limiter),
		metric: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      name + "_limiter_observations",
			Buckets:   buckets,
		}),
	}
}

func (l *deviceLimiters) reject(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	w.Header().Set("x-Permitted-Requests", fmt.Sprintf("%d per %s", l.max, l.window))
	http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
}

func (l *deviceLimiters) Serve(ctx context.Context) error {
	for {
		select {
		case <-time.After(time.Hour):
			cutoff := time.Now().Add(-time.Hour).UnixNano()
			l.mut.Lock()
			for k, v := range l.limiters {
				if v.starts < cutoff {
					delete(l.limiters, k)
				}
			}
			l.mut.Unlock()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (l *deviceLimiters) allow(d *protocol.DeviceID) (bool, time.Duration) {
	l.mut.Lock()
	defer l.mut.Unlock()

	lim, ok := l.limiters[*d]
	if !ok {
		lim = &limiter{}
		l.limiters[*d] = lim
	}
	if lim.allow(time.Now(), l.window, l.max, l.metric) {
		return true, 0
	}

	retryAfter := l.window * time.Duration(lim.cur) / time.Duration(l.max)
	return false, min(max(l.window, retryAfter), time.Hour)
}

// limiter implements a simple sliding-window rate limiter
type limiter struct {
	starts int64 // unix nanos
	cur    int32
	prev   int32
}

func (l *limiter) allow(when time.Time, window time.Duration, max int, metric prometheus.Histogram) bool {
	now := when.UnixNano()
	switch {
	case now-l.starts >= 2*window.Nanoseconds():
		// state is entirely cleared
		l.starts = now
		l.prev = 0
		l.cur = 1
		if metric != nil {
			metric.Observe(0)
		}
		return true

	case now-l.starts >= window.Nanoseconds():
		// window has advanced
		l.starts += window.Nanoseconds()
		l.prev = l.cur
		l.cur = 1
		if metric != nil {
			metric.Observe(float64(l.prev))
		}

	default:
		// we're in the current window
		l.cur++
	}

	// Determine if the event should have been allowed
	f := float64(now-l.starts) / float64(window.Nanoseconds())
	events := int(float64(l.cur) + (1-f)*float64(l.prev))
	return events <= max
}
