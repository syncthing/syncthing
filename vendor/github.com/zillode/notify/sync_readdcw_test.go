// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build windows

package notify

import (
	"syscall"
	"time"
	"unsafe"
)

var modkernel32 = syscall.NewLazyDLL("kernel32.dll")
var procSetSystemFileCacheSize = modkernel32.NewProc("SetSystemFileCacheSize")
var zero = uintptr(1<<(unsafe.Sizeof(uintptr(0))*8) - 1)

func Sync() {
	// TODO(pknap): does not work without admin privileges, but I'm going
	// to hack it.
	// r, _, err := procSetSystemFileCacheSize.Call(none, none, 0)
	// if r == 0 {
	//   dbgprint("SetSystemFileCacheSize error:", err)
	// }
}

// UpdateWait pauses the program for some minimal amount of time. This function
// is required only by implementations which work asynchronously. It gives
// watcher structure time to update its internal state.
func UpdateWait() {
	time.Sleep(50 * time.Millisecond)
}
