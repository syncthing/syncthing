// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io"
	"io/ioutil"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoatime(t *testing.T) {
	f, err := ioutil.TempFile("", "syncthing-testFs-")
	ok(t, err)

	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	// Only run this test on common filesystems that support O_NOATIME.
	// On others, we may not get an error.
	if !supportsNoatime(t, f) {
		t.Log("temp directory may not support O_NOATIME, skipping")
		t.Skip()
	}
	// From this point on, we own the file, so we should not get EPERM.

	_, err = io.WriteString(f, "Hello!")
	ok(t, err)
	_, err = f.Seek(0, io.SeekStart)
	ok(t, err)

	getAtime := func() syscall.Timespec {
		info, err := f.Stat()
		ok(t, err)
		st := info.Sys().(*syscall.Stat_t)
		return st.Atim
	}

	atime := getAtime()

	err = setNoatime(f)
	ok(t, err)

	_, err = f.Read(make([]byte, 1))
	ok(t, err)
	if newAtime := getAtime(); newAtime != atime {
		t.Fatal("atime updated despite O_NOATIME")
	}
}

func supportsNoatime(t *testing.T, f *os.File) bool {
	var fsinfo unix.Statfs_t
	err := unix.Fstatfs(int(f.Fd()), &fsinfo)
	ok(t, err)

	return fsinfo.Type == unix.BTRFS_SUPER_MAGIC ||
		fsinfo.Type == unix.EXT2_SUPER_MAGIC ||
		fsinfo.Type == unix.EXT3_SUPER_MAGIC ||
		fsinfo.Type == unix.EXT4_SUPER_MAGIC ||
		fsinfo.Type == unix.TMPFS_MAGIC
}
