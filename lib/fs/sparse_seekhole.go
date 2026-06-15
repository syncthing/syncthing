// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build linux || darwin || freebsd

package fs

import "golang.org/x/sys/unix"

// SetSparse marks a file sparse so unwritten regions become holes. No-op here:
// Truncate already does this on sparse-capable filesystems (ext4, APFS, UFS).
// Where it doesn't (HFS+), NextHole finds no hole and the caller refuses reuse.
func SetSparse(File) {}

// NextHole returns the next hole at or after offset via SEEK_HOLE (see
// lseek(2)). EOF counts as a hole, so a fully allocated file, or any file on a
// filesystem without hole support, returns its size. False only if SEEK_HOLE
// fails. Uses Seek, so it moves the file offset; pass a dedicated descriptor.
func NextHole(f File, offset int64) (int64, bool) {
	pos, err := f.Seek(offset, unix.SEEK_HOLE)
	if err != nil {
		return 0, false
	}
	return pos, true
}
