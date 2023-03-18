// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

// (No syscall.Umask or the equivalent on Windows)

package osutil

import (
	"os"
	"syscall"
	"testing"
)

func TestTempFilePermissions(t *testing.T) {
	// Set a zero umask, so any files created will have the permission bits
	// asked for in the create call and nothing less.
	oldMask := syscall.Umask(0)
	defer syscall.Umask(oldMask)

	fd, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}

	info, err := fd.Stat()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fd.Name())
	defer fd.Close()

	// The temp file should have 0600 permissions at the most, or we have a
	// security problem in CreateAtomic.
	t.Logf("Got 0%03o", info.Mode())
	if info.Mode()&^0600 != 0 {
		t.Errorf("Permission 0%03o is too generous", info.Mode())
	}
}
