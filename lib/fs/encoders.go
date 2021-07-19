// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

func (t FilesystemEncoderType) String() string {
	switch t {
	case FilesystemEncoderTypeDefault:
		return "default"
	case FilesystemEncoderTypeAuto:
		return "auto"
	case FilesystemEncoderTypeWindows:
		return "windows"
	case FilesystemEncoderTypeAndroid:
		return "android"
	case FilesystemEncoderTypeIos:
		return "ios"
	case FilesystemEncoderTypePlan9:
		return "plan9"
	case FilesystemEncoderTypeSafe:
		return "safe"
	// case FilesystemEncoderTypeCustom:
	//   return "custom"
	default:
		return "unknown"
	}

}

func (t FilesystemEncoderType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *FilesystemEncoderType) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "default":
		*t = FilesystemEncoderTypeDefault
	case "auto":
		*t = FilesystemEncoderTypeAuto
	case "windows":
		*t = FilesystemEncoderTypeWindows
	case "android":
		*t = FilesystemEncoderTypeAndroid
	case "ios":
		*t = FilesystemEncoderTypeIos
	case "plan9":
		*t = FilesystemEncoderTypePlan9
	case "safe":
		*t = FilesystemEncoderTypeSafe
		// case "custom":
		//   t = FilesystemEncoderTypeCustom
	default:
		*t = FilesystemEncoderTypeDefault
	}
	return nil
}
