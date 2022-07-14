// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"github.com/syncthing/syncthing/lib/protocol"
)

// An OSDataGetter is responsible for returing a new set of OS data, based
// on both the current FileInfo and a new stat() FileInfo. This is a
// separate interface from Filesystem because the same implementation can in
// some cases be used on different filesystems (e.g., the POSIX one works on
// both Basic and Fake).
type OSDataGetter interface {
	// GetOSData returns a map with the current operating system private
	// data for the current operating system only. It does not need to
	// return entries for other operating systems. Generally this will be a
	// one-entry map, but it's also imaginable that it'll return an entry
	// for POSIX and another entry for Linux with more specific data...
	GetOSData(cur *protocol.FileInfo, stat FileInfo) (protocol.PlatformData, error)
}
