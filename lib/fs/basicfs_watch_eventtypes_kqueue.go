// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build dragonfly || freebsd || netbsd || openbsd
// +build dragonfly freebsd netbsd openbsd

package fs

import "github.com/syncthing/notify"

const (
	// Platform independent notify.Create is required, as kqueue does not have
	// any event signalling file creation, but notify does generate those internally.
	// NoteAttrib is not only required for permissions, but also mod. time changes
	subEventMask  = notify.NoteDelete | notify.NoteWrite | notify.NoteRename | notify.Create | notify.NoteAttrib | notify.NoteExtend
	permEventMask = 0
	rmEventMask   = notify.NoteDelete | notify.NoteRename
)
