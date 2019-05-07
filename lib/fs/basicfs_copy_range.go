// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
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

type copyRangeFuncGeneric func(src, dst File, srcOffset, dstOffset, size int64) error
type copyRangeFuncOptimised func(src, dst basicFile, srcOffset, dstOffset, size int64) error

type copyRangeImplementation struct {
	name string
	impl copyRangeFuncGeneric
}

func registerCopyRangeImplementation(impl copyRangeImplementation) {
	mut.Lock()
	defer mut.Unlock()

	for _, implName := range implementationOrder {
		if implName == impl.name {
			l.Debugln("Registering " + impl.name + " copyRange implementation")
			copyRangeImplementations = append(copyRangeImplementations, impl)

			sort.Slice(copyRangeImplementations, func(i, j int) bool {
				iidx := util.StringIndex(implementationOrder, copyRangeImplementations[i].name)
				jidx := util.StringIndex(implementationOrder, copyRangeImplementations[j].name)
				return iidx < jidx
			})

			return
		}
	}

	l.Debugln("Discarding " + impl.name + " copyRange implementation")
}

// CopyRange tries to use the most optimal way to copy data between two files.
// Takes size bytes at offset srcOffset from the source file, and copies the data to destination file at offset
// dstOffset. If required, adjusts the size of the destination file to fit that much data.
//
// On unix, uses ref-linking if the underlying copy-on-write filesystem supports it (tested on xfs and btrfs),
// which referencing existing data in the source file, instead of making a copy and taking up additional space.
//
// CopyRange does it best to have no effect on src and dst file offsets (copy operation should not affect it).
func CopyRange(src, dst File, srcOffset, dstOffset, size int64) error {
	if len(copyRangeImplementations) == 0 {
		panic("bug: no CopyRange implementations")
	}
	ss, ser := src.Stat()
	ds, der := dst.Stat()
	if ser != nil {
		l.Infoln("ser:" + ser.Error())
	} else {
		fmt.Print(ss.Size())
	}
	if der != nil {
		l.Infoln("ser:" + der.Error())
	} else {
		fmt.Println(ds.Size())
	}
	l.Infof("copy range %s to %s (%d to %d, %d bytes)\n", src.Name(), dst.Name(), srcOffset, dstOffset, size)
	var err error
	for _, copier := range copyRangeImplementations {
		if err = copier.impl(src, dst, srcOffset, dstOffset, size); err == nil {
			l.Infof(copier.name + " succeeded")
			return nil
		}
		l.Infoln("copy range " + copier.name + " err " + err.Error())
	}

	// Return the last error
	return err
}

func getCopyOptimisations() []string {
	opt := strings.ToLower(os.Getenv("STCOPYOPTIMISATIONS"))
	if opt == "" {
		// ioctl first because it's available on early kernels and works on btrfs
		// copy_file_range is only available on linux 4.5+ and works on xfs and btrfs
		// sendfile does not do any block reuse, but works since 2.6+ or so.
		opt = "ioctl,copy_file_range,sendfile,generic"
	}

	impls := strings.Split(opt, ",")

	if util.StringIndex(impls, "generic") == -1 {
		impls = append(impls, "generic")
	}

	return impls
}

func asGeneric(input copyRangeFuncOptimised) copyRangeFuncGeneric {
	return func(src, dst File, srcOffset, dstOffset, size int64) error {
		srcFile, srcOk := src.(basicFile)
		dstFile, dstOk := dst.(basicFile)
		if srcOk && dstOk {
			return input(srcFile, dstFile, srcOffset, dstOffset, size)
		}
		return syscall.ENOTSUP
	}
}
