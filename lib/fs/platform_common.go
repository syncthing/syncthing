// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os/user"
	"strconv"

	"github.com/syncthing/syncthing/lib/protocol"
)

// unixPlatformData is used on all platforms, because apart from being the
// implementation for BasicFilesystem on Unixes it's also the implementation
// in fakeFS.
func unixPlatformData(fs Filesystem, name string) (protocol.PlatformData, error) {
	stat, err := fs.Lstat(name)
	if err != nil {
		return protocol.PlatformData{}, err
	}

	ownerUID := stat.Owner()
	ownerName := ""
	if u, err := user.LookupId(strconv.Itoa(ownerUID)); err == nil {
		ownerName = u.Username
	}

	groupID := stat.Group()
	groupName := ""
	if g, err := user.LookupGroupId(strconv.Itoa(groupID)); err == nil {
		groupName = g.Name
	}

	return protocol.PlatformData{
		Unix: &protocol.UnixData{
			OwnerName: ownerName,
			GroupName: groupName,
			UID:       ownerUID,
			GID:       groupID,
		},
	}, nil
}
