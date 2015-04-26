// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build freebsd openbsd dragonfly

package main

import "errors"

func memorySize() (int64, error) {
	return 0, errors.New("not implemented")
}
