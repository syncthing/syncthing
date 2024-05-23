// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build darwin && !kqueue && cgo
// +build darwin,!kqueue,cgo

package fs

import "github.com/syncthing/notify"

const (
	subEventMask = notify.Create | notify.Remove | notify.Write | notify.Rename | notify.FSEventsInodeMetaMod | notify.FSEventsXattrMod
	// FSEventsChangeOwner fires on permission change
	permEventMask = notify.FSEventsChangeOwner
	rmEventMask   = notify.Remove | notify.Rename
)
