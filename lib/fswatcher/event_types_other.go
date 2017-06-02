// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !linux,!windows,!dragonfly,!freebsd,!netbsd,!openbsd,!solaris
// +build !darwin darwin,cgo

// Catch all platforms that are not specifically handled to use the generic
// event types.

package fswatcher

import "github.com/zillode/notify"

func (w *watcher) eventMask() notify.Event {
	return notify.All
}

func (w *watcher) removeEventMask() notify.Event {
	return notify.Remove | notify.Rename
}
