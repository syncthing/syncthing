// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build freebsd || openbsd || dragonfly
// +build freebsd openbsd dragonfly

package ur

func memorySize() int64 {
	return 0
}
