// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package config

type FolderType int

const (
	FolderTypeReadWrite FolderType = iota // default is readwrite
	FolderTypeReadOnly
)

func (t FolderType) String() string {
	switch t {
	case FolderTypeReadWrite:
		return "readwrite"
	case FolderTypeReadOnly:
		return "readonly"
	default:
		return "unknown"
	}
}

func (t FolderType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *FolderType) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "readwrite":
		*t = FolderTypeReadWrite
	case "readonly":
		*t = FolderTypeReadOnly
	default:
		*t = FolderTypeReadWrite
	}
	return nil
}
