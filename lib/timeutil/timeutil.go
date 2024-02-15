// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package timeutil

import "time"

// StopAndDrain stops the timer and ensures the channel is drained. Must not be
// called concurrently with receiving from the timer channel.
func StopAndDrain(t *time.Timer) {
	if !t.Stop() {
		<-t.C
	}
}

// ResetTimer is timer.Stop()+timer.Reset() to properly reset the timer
// according to the pattern mandated by https://pkg.go.dev/time#Timer.Reset:
// timers must only be reset if they are stopped and drained. If you're in a
// branch that just received from the timer channel you can use
// timer.Reset() directly, otherwise this pattern must be used.
func ResetTimer(t *time.Timer, dur time.Duration) {
	StopAndDrain(t)
	t.Reset(dur)
}
