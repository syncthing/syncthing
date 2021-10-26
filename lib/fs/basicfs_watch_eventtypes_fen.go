// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build solaris && cgo
// +build solaris,cgo

package fs

import "github.com/syncthing/notify"

const (
	subEventMask  = notify.Create | notify.FileModified | notify.FileRenameFrom | notify.FileDelete | notify.FileRenameTo
	permEventMask = notify.FileAttrib
	rmEventMask   = notify.FileDelete | notify.FileRenameFrom
)
