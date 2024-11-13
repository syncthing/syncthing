// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type FilesystemType int32

const (
	FilesystemTypeBasic FilesystemType = 0
	FilesystemTypeFake  FilesystemType = 1
)

func (t FilesystemType) String() string {
	switch t {
	case FilesystemTypeBasic:
		return "basic"
	case FilesystemTypeFake:
		return "fake"
	default:
		return "unknown"
	}
}
