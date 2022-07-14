// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package model

import (
	"os/user"
	"strconv"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (f *sendReceiveFolder) syncOwnership(file *protocol.FileInfo, path string) error {
	if file.Platform.Posix == nil {
		// No owner data, nothing to do
		return nil
	}

	// Try to look up the user and group by name, defaulting to the
	// numerical UID and GID if there is no match.

	uid := strconv.Itoa(file.Platform.Posix.UID)
	if file.Platform.Posix.OwnerName != "" {
		us, err := user.Lookup(file.Platform.Posix.OwnerName)
		if err == nil && us.Uid != "" {
			uid = us.Uid
		}
	}

	gid := strconv.Itoa(file.Platform.Posix.GID)
	if file.Platform.Posix.GroupName != "" {
		gr, err := user.LookupGroup(file.Platform.Posix.GroupName)
		if err == nil && gr.Gid != "" {
			gid = gr.Gid
		}
	}

	return f.mtimefs.Lchown(path, uid, gid)
}
