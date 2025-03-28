// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package fs

import "github.com/syncthing/syncthing/lib/protocol"

func (f *BasicFilesystem) PlatformData(name string, scanOwnership, scanXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error) {
	return unixPlatformData(f, name, f.userCache, f.groupCache, scanOwnership, scanXattrs, xattrFilter)
}
