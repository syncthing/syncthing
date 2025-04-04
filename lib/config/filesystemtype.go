// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "github.com/syncthing/syncthing/lib/fs"

type FilesystemType string

const (
	FilesystemTypeBasic FilesystemType = "basic"
	FilesystemTypeFake  FilesystemType = "fake"
)

func (t FilesystemType) ToFS() fs.FilesystemType {
	if t == "" {
		// legacy compat, zero value means basic
		return fs.FilesystemTypeBasic
	}
	return fs.FilesystemType(string(t))
}

func (t FilesystemType) String() string {
	if t == "" {
		// legacy compat, zero value means basic
		return string(FilesystemTypeBasic)
	}
	return string(t)
}

func (t FilesystemType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *FilesystemType) UnmarshalText(bs []byte) error {
	if len(bs) == 0 {
		// legacy compat, zero value means basic
		*t = FilesystemTypeBasic
		return nil
	}
	*t = FilesystemType(string(bs))
	return nil
}

func (t *FilesystemType) ParseDefault(str string) error {
	return t.UnmarshalText([]byte(str))
}
