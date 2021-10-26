// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !linux && !windows && !dragonfly && !freebsd && !netbsd && !openbsd && !solaris && !darwin && !cgo && !ios
// +build !linux,!windows,!dragonfly,!freebsd,!netbsd,!openbsd,!solaris,!darwin,!cgo,!ios

// Catch all platforms that are not specifically handled to use the generic
// event types.

package fs

import "github.com/syncthing/notify"

const (
	subEventMask  = notify.All
	permEventMask = 0
	rmEventMask   = notify.Remove | notify.Rename
)
