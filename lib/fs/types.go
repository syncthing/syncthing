// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type FilesystemType int

const (
	FilesystemTypeBasic FilesystemType = iota
	FilesystemTypeFake
	FilesystemTypeCaseBasic // default
)

func (t FilesystemType) String() string {
	switch t {
	case FilesystemTypeBasic:
		return "basic"
	case FilesystemTypeFake:
		return "fake"
	case FilesystemTypeCaseBasic:
		return "casebasic"
	default:
		return "unknown"
	}
}

func (t FilesystemType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *FilesystemType) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "basic":
		*t = FilesystemTypeBasic
	case "fake":
		*t = FilesystemTypeFake
	case "casebasic":
		*t = FilesystemTypeCaseBasic
	default:
		*t = FilesystemTypeBasic
	}
	return nil
}

func (t *FilesystemType) ParseDefault(v string) error {
	return t.UnmarshalText([]byte(v))
}
