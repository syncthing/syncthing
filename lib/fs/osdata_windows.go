// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"os/user"

	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/sys/windows"
)

func NewOSDataGetter(underlying Filesystem) OSDataGetter {
	return &WindowsOSDataGetter{fs: underlying}
}

type WindowsOSDataGetter struct {
	fs Filesystem
}

func (p *WindowsOSDataGetter) GetOSData(cur *protocol.FileInfo, stat FileInfo) (map[protocol.OS][]byte, error) {
	// The underlying filesystem must be a BasicFilesystem, because we're
	// going to assume the file is an object on local disk and make system
	// calls on it.
	basic, ok := p.fs.(*BasicFilesystem)
	if !ok {
		return nil, fmt.Errorf("underlying filesystem is not a BasicFilesystem")
	}

	rootedName, err := basic.rooted(cur.Name)
	if err != nil {
		return nil, err
	}
	hdl, err := windows.Open(rootedName, windows.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer windows.Close(hdl)

	// GetSecurityInfo returns an owner SID.
	sd, err := windows.GetSecurityInfo(hdl, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return nil, err
	}

	// The owner SID might represent a user or a group. We try to look it up
	// as both, and set the appropriate fields in the OS data.
	pd := &protocol.WindowsOSData{}
	if us, err := user.LookupId(owner.String()); err == nil {
		pd.OwnerName = us.Username
	} else if gr, err := user.LookupGroupId(owner.String()); err == nil {
		pd.OwnerName = gr.Name
		pd.OwnerIsGroup = true
	} else {
		l.Debugf("Failed to resolve owner for %s: %v", rootedName, err)
	}

	bs, err := pd.Marshal()
	if err != nil {
		return nil, fmt.Errorf("surprising error marshalling private data: %w", err)
	}

	l.Debugln("OS data for", rootedName, "is", pd)
	return map[protocol.OS][]byte{
		protocol.OsWindows: bs,
	}, nil
}
