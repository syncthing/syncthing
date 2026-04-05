// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"fmt"
	"strings"

	"github.com/syncthing/syncthing/lib/protocol"
)

type Counts struct {
	Files       int
	Directories int
	Symlinks    int
	Deleted     int
	Bytes       int64
	Sequence    int64              // zero for the global state
	DeviceID    protocol.DeviceID  // device ID for remote devices, or special values for local/global
	LocalFlags  protocol.FlagLocal // the local flag for this count bucket
}

func (c Counts) Add(other Counts) Counts {
	return Counts{
		Files:       c.Files + other.Files,
		Directories: c.Directories + other.Directories,
		Symlinks:    c.Symlinks + other.Symlinks,
		Deleted:     c.Deleted + other.Deleted,
		Bytes:       c.Bytes + other.Bytes,
		Sequence:    c.Sequence + other.Sequence,
		DeviceID:    protocol.EmptyDeviceID,
		LocalFlags:  c.LocalFlags | other.LocalFlags,
	}
}

func (c Counts) TotalItems() int {
	return c.Files + c.Directories + c.Symlinks + c.Deleted
}

func (c Counts) String() string {
	var flags strings.Builder
	if c.LocalFlags&protocol.FlagLocalNeeded != 0 {
		flags.WriteString("Need")
	}
	if c.LocalFlags&protocol.FlagLocalIgnored != 0 {
		flags.WriteString("Ignored")
	}
	if c.LocalFlags&protocol.FlagLocalMustRescan != 0 {
		flags.WriteString("Rescan")
	}
	if c.LocalFlags&protocol.FlagLocalReceiveOnly != 0 {
		flags.WriteString("Recvonly")
	}
	if c.LocalFlags&protocol.FlagLocalUnsupported != 0 {
		flags.WriteString("Unsupported")
	}
	if c.LocalFlags != 0 {
		flags.WriteString(fmt.Sprintf("(%x)", c.LocalFlags))
	}
	if flags.Len() == 0 {
		flags.WriteString("---")
	}
	return fmt.Sprintf("{Device:%v, Files:%d, Dirs:%d, Symlinks:%d, Del:%d, Bytes:%d, Seq:%d, Flags:%s}", c.DeviceID, c.Files, c.Directories, c.Symlinks, c.Deleted, c.Bytes, c.Sequence, flags.String())
}

// Equal compares the numbers only, not sequence/dev/flags.
func (c Counts) Equal(o Counts) bool {
	return c.Files == o.Files && c.Directories == o.Directories && c.Symlinks == o.Symlinks && c.Deleted == o.Deleted && c.Bytes == o.Bytes
}
