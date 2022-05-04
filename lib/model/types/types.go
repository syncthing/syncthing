// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package types

// A []FileError is sent as part of an event and will be JSON serialized.
type FileError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}
