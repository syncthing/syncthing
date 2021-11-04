// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package osutil

import (
	"errors"
)

func MaximizeOpenFileLimit() (int, error) {
	return 0, errors.New("not relevant on Windows")
}
