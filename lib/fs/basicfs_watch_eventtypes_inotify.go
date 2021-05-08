// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build linux

package fs

import "github.com/syncthing/notify"

// notify.InAttrib is not only required for permissions, but also mod. time changes
const (
	subEventMask  = notify.InCreate | notify.InMovedTo | notify.InDelete | notify.InDeleteSelf | notify.InModify | notify.InMovedFrom | notify.InMoveSelf | notify.InAttrib
	permEventMask = 0
	rmEventMask   = notify.InDelete | notify.InDeleteSelf | notify.InMovedFrom | notify.InMoveSelf
)
