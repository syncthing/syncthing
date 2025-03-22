// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package timeutil

import (
	"sync/atomic"
	"time"
)

var prevNanos atomic.Int64

// StrictlyMonotonicNanos returns the current time in Unix nanoseconds.
// Guaranteed to strictly increase for each call, regardless of the
// underlying OS timer resolution or clock jumps.
func StrictlyMonotonicNanos() int64 {
	for {
		old := prevNanos.Load()
		now := max(time.Now().UnixNano(), old+1)
		if prevNanos.CompareAndSwap(old, now) {
			return now
		}
	}
}
