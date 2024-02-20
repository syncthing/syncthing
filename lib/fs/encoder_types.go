// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

func (t FilesystemEncoderType) String() string {
	switch t {
	case FilesystemEncoderTypePassthrough:
		return "passthrough"
	case FilesystemEncoderTypeStandard:
		return "standard"
	case FilesystemEncoderTypeFat:
		return "fat"
	default:
		return "unknown"
	}
}

func (t FilesystemEncoderType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *FilesystemEncoderType) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "passthrough":
		*t = FilesystemEncoderTypePassthrough
	case "standard":
		*t = FilesystemEncoderTypeStandard
	case "fat":
		*t = FilesystemEncoderTypeFat
	default:
		*t = DefaultFilesystemEncoderType
	}
	return nil
}

func (t *FilesystemEncoderType) ParseDefault(str string) error {
	return t.UnmarshalText([]byte(str))
}
