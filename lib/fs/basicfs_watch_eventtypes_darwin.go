// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build darwin && !kqueue && cgo && !ios
// +build darwin,!kqueue,cgo,!ios

package fs

import "github.com/syncthing/notify"

const (
	subEventMask  = notify.Create | notify.Remove | notify.Write | notify.Rename | notify.FSEventsInodeMetaMod
	permEventMask = 0
	rmEventMask   = notify.Remove | notify.Rename
)
