// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
)

var (
	copyRangeImplementations []copyRangeImplementation
	mut                      = sync.NewMutex()
	implementationOrder      = getCopyOptimisations() // This runs before init
)

type copyRangeImplementation struct {
	name string
	impl func(src, dst basicFile, srcOffset, dstOffset, size int64) error
}

func registerCopyRangeImplementation(impl copyRangeImplementation) {
	mut.Lock()
	defer mut.Unlock()

	if util.StringIndex(implementationOrder, impl.name) == -1 {
		l.Debugln("Discarding " + impl.name + " copyRange implementation")
		return
	}

	l.Debugln("Registering " + impl.name + " copyRange implementation")
	copyRangeImplementations = append(copyRangeImplementations, impl)

	sort.Slice(copyRangeImplementations, func(i, j int) bool {
		iidx := util.StringIndex(implementationOrder, copyRangeImplementations[i].name)
		jidx := util.StringIndex(implementationOrder, copyRangeImplementations[j].name)
		return iidx < jidx
	})
}

// CopyRange tries to use the most optimal way to copy data between two files.
// Takes size bytes at offset srcOffset from the source file, and copies the data to destination file at offset
// dstOffset. If required, adjusts the size of the destination file to fit that much data.
//
// On Linux/BSD it tries to use ioctl and copy_file_range system calls, which if the underlying filesystem supports it
// tries referencing existing data in the source file, instead of making a copy and taking up additional space.
//
// If that is not possible, the data will be copied using an in-kernel copy (copy_file_range fallback, sendfile),
// oppose to user space copy, if those system calls are available and supported for the source and target in question.
//
// CopyRange does it's best to have no effect on src and dst file offsets (copy operation should not affect it).
func CopyRange(src, dst File, srcOffset, dstOffset, size int64) error {
	if len(copyRangeImplementations) == 0 {
		return syscall.ENOTSUP
	}

	srcFile, srcOk := src.(basicFile)
	dstFile, dstOk := dst.(basicFile)
	if !srcOk || !dstOk {
		return syscall.ENOTSUP
	}

	var err error
	for _, copier := range copyRangeImplementations {
		if err = copier.impl(srcFile, dstFile, srcOffset, dstOffset, size); err == nil {
			return nil
		}
	}

	// Return the last error
	return err
}

func getCopyOptimisations() []string {
	opt := strings.ToLower(os.Getenv("STCOPYOPTIMISATIONS"))
	if opt == "" {
		// ioctl first because it's available on early kernels and works on btrfs
		// copy_file_range is only available on linux 4.5+ and works on xfs and btrfs
		// sendfile does not do any block reuse on normal filesystems but might re, but works since 2.6+ or so, and having a in-kernel copy is more
		// efficient for NFS/CIFS and large files.
		opt = "ioctl,copy_file_range,sendfile"
	}

	return strings.Split(opt, ",")
}
