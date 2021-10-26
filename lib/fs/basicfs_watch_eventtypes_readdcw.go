// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package fs

import "github.com/syncthing/notify"

const (
	subEventMask  = notify.FileNotifyChangeFileName | notify.FileNotifyChangeDirName | notify.FileNotifyChangeSize | notify.FileNotifyChangeCreation | notify.FileNotifyChangeLastWrite
	permEventMask = notify.FileNotifyChangeAttributes
	rmEventMask   = notify.FileActionRemoved | notify.FileActionRenamedOldName
)
