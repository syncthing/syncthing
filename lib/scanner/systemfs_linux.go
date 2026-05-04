// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build linux
// +build linux

package scanner

import (
	"os"
	"syscall"
)

// isSystemFilesystem checks if a path is mounted on a system filesystem
// (procfs, sysfs, devtmpfs, etc.) that should not be recursively scanned.
// Only applies to Linux.
func isSystemFilesystem(path string) bool {
	var statfs syscall.Statfs_t
	if err := syscall.Statfs(path, &statfs); err != nil {
		// If we can't stat, assume it's not a system filesystem
		return false
	}

	// Check against known system filesystem type magic numbers
	// These are defined in Linux kernel include/linux/magic.h
	switch uint32(statfs.Type) {
	case 0x9FA1:     // PROC_SUPER_MAGIC - procfs
		return true
	case 0x62656572: // SYSFS_MAGIC - sysfs
		return true
	case 0x1021994:  // TMPFS_MAGIC - devtmpfs
		return true
	case 0x2:        // MINIX_SUPER_MAGIC (not really system but skip as safety)
		// We only skip well-known system filesystems
		return false
	}

	return false
}
