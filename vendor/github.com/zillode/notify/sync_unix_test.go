// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build !windows

package notify

import "golang.org/x/sys/unix"

func Sync() {
	unix.Sync()
}

// UpdateWait is required only by windows watcher implementation. On other
// platforms this function is no-op.
func UpdateWait() {
}
