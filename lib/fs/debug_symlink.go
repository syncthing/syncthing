// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import (
	"os"
	"testing"
)

// DebugSymlinkForTestsOnly is not and should not be used in Syncthing code,
// hence the cumbersome name to make it obvious if this ever leaks. Its
// reason for existence is the Windows version, which allows creating
// symlinks when non-elevated.
func DebugSymlinkForTestsOnly(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatal(err)
	}
}
