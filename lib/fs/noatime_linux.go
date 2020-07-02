// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build linux

package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

// SetNoatime tries to set the O_NOATIME flag on f, which prevents the kernel
// from updating the atime on a read call.
//
// The call fails when we're not the owner of the file or root. The caller
// should ignore the error, which is returned for testing only.
func setNoatime(f *os.File) error {
	fd := f.Fd()
	flags, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err == nil {
		_, err = unix.FcntlInt(fd, unix.F_SETFL, flags|unix.O_NOATIME)
	}
	return err
}
