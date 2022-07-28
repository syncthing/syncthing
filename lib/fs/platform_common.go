// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os/user"
	"runtime"
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
	if u, err := user.LookupId(strconv.Itoa(ownerUID)); err == nil && u.Username != "" {
		ownerName = u.Username
	} else if ownerUID == 0 {
		// We couldn't look up a name, but UID zero should be "root". This
		// fixup works around the (unlikely) situation where the ownership
		// is 0:0 but we can't look up a name for either uid zero or gid
		// zero. If that were the case we'd return a zero PlatformData which
		// wouldn't get serialized over the wire and the other side would
		// assume a lack of ownership info...
		ownerName = "root"
	}

	groupID := stat.Group()
	groupName := ""
	if g, err := user.LookupGroupId(strconv.Itoa(groupID)); err == nil && g.Name != "" {
		groupName = g.Name
	} else if groupID == 0 {
		groupName = "root"
	}

	pd := protocol.PlatformData{
		Unix: &protocol.UnixData{
			OwnerName: ownerName,
			GroupName: groupName,
			UID:       ownerUID,
			GID:       groupID,
		},
	}

	xattrs, err := fs.GetXattr(name)
	if err != nil {
		return protocol.PlatformData{}, err
	}
	switch runtime.GOOS {
	case "linux":
		pd.Linux = &protocol.XattrData{Xattrs: xattrs}
	case "macos":
		pd.MacOS = &protocol.XattrData{Xattrs: xattrs}
	case "freebsd":
		pd.MacOS = &protocol.XattrData{Xattrs: xattrs}
	case "netbsd":
		pd.NetBSD = &protocol.XattrData{Xattrs: xattrs}
	case "openbsd":
		pd.OpenBSD = &protocol.XattrData{Xattrs: xattrs}
	case "illumos", "solaris":
		pd.Illumos = &protocol.XattrData{Xattrs: xattrs}
	}

	return pd, nil
}
