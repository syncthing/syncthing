// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package osutil

// CheckNameConflict returns true if every path component of name up to and
// including filepath.Join(base, name) doesn't conflict with any existing
// files or folders with different names.
func CheckNameConflict(base, name string) bool {
	return true
}
