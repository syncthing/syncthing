// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ios
// +build ios

package osutil

// SetLowPriority not possible on some platforms
// I/O priority depending on the platform and OS.
func SetLowPriority() error {
	return nil
}
