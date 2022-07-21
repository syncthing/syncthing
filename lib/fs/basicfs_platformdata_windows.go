// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os/user"

	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/sys/windows"
)

func (f *BasicFilesystem) PlatformData(name string) (protocol.PlatformData, error) {
	rootedName, err := f.rooted(name)
	if err != nil {
		return protocol.PlatformData{}, err
	}
	hdl, err := windows.Open(rootedName, windows.O_RDONLY, 0)
	if err != nil {
		return protocol.PlatformData{}, err
	}
	defer windows.Close(hdl)

	// GetSecurityInfo returns an owner SID.
	sd, err := windows.GetSecurityInfo(hdl, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return protocol.PlatformData{}, err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return protocol.PlatformData{}, err
	}

	// The owner SID might represent a user or a group. We try to look it up
	// as both, and set the appropriate fields in the OS data.
	pd := &protocol.WindowsData{}
	if us, err := user.LookupId(owner.String()); err == nil {
		pd.OwnerName = us.Username
	} else if gr, err := user.LookupGroupId(owner.String()); err == nil {
		pd.OwnerName = gr.Name
		pd.OwnerIsGroup = true
	} else {
		l.Debugf("Failed to resolve owner for %s: %v", rootedName, err)
	}

	return protocol.PlatformData{Windows: pd}, nil
}
