// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncutil

import (
	"sync"
	"time"
)

// TimeoutCond is a variant on Cond. It has roughly the same semantics regarding 'L' - it must be held
// both when broadcasting and when calling TimeoutCondWaiter.Wait()
// Call Broadcast() to broadcast to all waiters on the TimeoutCond. Call SetupWait to create a
// TimeoutCondWaiter configured with the given timeout, which can then be used to listen for
// broadcasts.
type TimeoutCond struct {
	L  sync.Locker
	ch chan struct{}
}

// TimeoutCondWaiter is a type allowing a consumer to wait on a TimeoutCond with a timeout. Wait() may be called multiple times,
// and will return true every time that the TimeoutCond is broadcast to. Once the configured timeout
// expires, Wait() will return false.
// Call Stop() to release resources once this TimeoutCondWaiter is no longer needed.
type TimeoutCondWaiter struct {
	c     *TimeoutCond
	timer *time.Timer
}

func NewTimeoutCond(l sync.Locker) *TimeoutCond {
	return &TimeoutCond{
		L: l,
	}
}

func (c *TimeoutCond) Broadcast() {
	// ch.L must be locked when calling this function

	if c.ch != nil {
		close(c.ch)
		c.ch = nil
	}
}

func (c *TimeoutCond) SetupWait(timeout time.Duration) *TimeoutCondWaiter {
	timer := time.NewTimer(timeout)

	return &TimeoutCondWaiter{
		c:     c,
		timer: timer,
	}
}

func (w *TimeoutCondWaiter) Wait() bool {
	// ch.L must be locked when calling this function

	// Ensure that the channel exists, since we're going to be waiting on it
	if w.c.ch == nil {
		w.c.ch = make(chan struct{})
	}
	ch := w.c.ch

	w.c.L.Unlock()
	defer w.c.L.Lock()

	select {
	case <-w.timer.C:
		return false
	case <-ch:
		return true
	}
}

func (w *TimeoutCondWaiter) Stop() {
	w.timer.Stop()
}
