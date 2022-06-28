// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"os/user"
	"strconv"

	"github.com/syncthing/syncthing/lib/protocol"
)

func NewPOSIXDataGetter(_ Filesystem) OSDataGetter {
	return &POSIXOSDataGetter{}
}

type POSIXOSDataGetter struct{}

func (p *POSIXOSDataGetter) GetOSData(_ *protocol.FileInfo, stat FileInfo) (map[protocol.OS][]byte, error) {
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

	// Create the POSIX private data structure and store it marshalled.
	pd := &protocol.POSIXOSData{
		OwnerName: ownerName,
		GroupName: groupName,
		UID:       ownerUID,
		GID:       groupID,
	}
	bs, err := pd.Marshal()
	if err != nil {
		return nil, fmt.Errorf("surprising error marshalling private data: %w", err)
	}
	return map[protocol.OS][]byte{
		protocol.OsPosix: bs,
	}, nil
}
