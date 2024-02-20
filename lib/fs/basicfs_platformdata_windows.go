// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/sys/windows"
)

func (f *BasicFilesystem) PlatformData(name string, scanOwnership, _ bool, _ XattrFilter) (protocol.PlatformData, error) {
	if !scanOwnership {
		// That's the only thing we do, currently
		return protocol.PlatformData{}, nil
	}

	rootedName, err := f.rooted(name, "chown")
	if err != nil {
		return protocol.PlatformData{}, fmt.Errorf("rooted for %s: %w", name, err)
	}
	hdl, err := openReadOnlyWithBackupSemantics(rootedName)
	if err != nil {
		return protocol.PlatformData{}, fmt.Errorf("open %s: %w", rootedName, err)
	}
	defer windows.Close(hdl)

	// GetSecurityInfo returns an owner SID.
	sd, err := windows.GetSecurityInfo(hdl, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return protocol.PlatformData{}, fmt.Errorf("get security info for %s: %w", rootedName, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return protocol.PlatformData{}, fmt.Errorf("get owner for %s: %w", rootedName, err)
	}

	pd := &protocol.WindowsData{}
	if us := f.userCache.lookup(owner.String()); us != nil {
		pd.OwnerName = us.Username
	} else if gr := f.groupCache.lookup(owner.String()); gr != nil {
		pd.OwnerName = gr.Name
		pd.OwnerIsGroup = true
	} else {
		l.Debugf("Failed to resolve owner for %s: %v", rootedName, err)
	}

	return protocol.PlatformData{Windows: pd}, nil
}

func openReadOnlyWithBackupSemantics(path string) (fd windows.Handle, err error) {
	// This is windows.Open but simplified to read-only only, and adding
	// FILE_FLAG_BACKUP_SEMANTICS which is required to open directories.
	if len(path) == 0 {
		return windows.InvalidHandle, windows.ERROR_FILE_NOT_FOUND
	}
	pathp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	var access uint32 = windows.GENERIC_READ
	var sharemode uint32 = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	var sa *windows.SecurityAttributes
	var createmode uint32 = windows.OPEN_EXISTING
	var attrs uint32 = windows.FILE_ATTRIBUTE_READONLY | windows.FILE_FLAG_BACKUP_SEMANTICS
	return windows.CreateFile(pathp, access, sharemode, sa, createmode, attrs, 0)
}
