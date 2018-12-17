// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build !darwin,!linux,!freebsd,!dragonfly,!netbsd,!openbsd,!windows
// +build !kqueue,!solaris

package notify

// Platform independent event values.
const (
	osSpecificCreate Event = 1 << iota
	osSpecificRemove
	osSpecificWrite
	osSpecificRename
	// internal
	// recursive is used to distinguish recursive eventsets from non-recursive ones
	recursive
	// omit is used for dispatching internal events; only those events are sent
	// for which both the event and the watchpoint has omit in theirs event sets.
	omit
)

var osestr = map[Event]string{}

type event struct{}

func (e *event) Event() (_ Event)         { return }
func (e *event) Path() (_ string)         { return }
func (e *event) Sys() (_ interface{})     { return }
func (e *event) isDir() (_ bool, _ error) { return }
