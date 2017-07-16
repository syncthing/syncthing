// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build dragonfly freebsd netbsd openbsd

package fswatcher

import "github.com/zillode/notify"

func (w *watcher) eventMask() notify.Event {
	events := notify.NoteDelete | notify.NoteWrite | notify.NoteRename
	if !w.folderCfg.IgnorePerms {
		events |= notify.NoteAttrib
	}
	return events
}

func (w *watcher) removeEventMask() notify.Event {
	return notify.NoteDelete | notify.NoteRename
}
