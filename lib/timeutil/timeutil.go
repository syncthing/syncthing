// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package timeutil

import "time"

// StopTimer stops the timer and ensures the channel is drained.
func StopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// ResetTimer is timer.Stop()+timer.Reset() to properly reset the timer
// according to the mandated pattern in https://pkg.go.dev/time#Timer.Reset:
// timers must only be reset if they are stopped and drained. If you're in a
// branch that just received from the timer channel you can use
// timer.Reset() directly, otherwise this pattern must be used.
func ResetTimer(t *time.Timer, dur time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(dur)
}

// StopTicker stops the ticker and drains the channel to ensure the ticker
// can be deallocated.
func StopTicker(t *time.Ticker) {
	t.Stop()
	select {
	case <-t.C:
	default:
	}
}
