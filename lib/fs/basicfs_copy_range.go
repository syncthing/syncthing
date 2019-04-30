// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io"
	"os"
	"strings"
)

// CopyRange tries to use the most optimal way to copy data between two files.
// Takes size bytes at offset srcOffset from the source file, and copies the data to destination file at offset
// dstOffset. If required, adjusts the size of the destination file to fit that much data.
//
// On unix, uses ref-linking if the underlying copy-on-write filesystem supports it (tested on xfs and btrfs),
// which referencing existing data in the source file, instead of making a copy and taking up additional space.
//
// CopyRange does it best to have no effect on src and dst file offsets (copy operation should not affect it).
func CopyRange(src, dst File, srcOffset, dstOffset, size int64) error {
	srcFile, srcOk := src.(basicFile)
	dstFile, dstOk := dst.(basicFile)
	if srcOk && dstOk {
		if err := copyRangeOptimised(srcFile, dstFile, srcOffset, dstOffset, size); err == nil {
			return nil
		}
	}

	return copyRangeGeneric(src, dst, srcOffset, dstOffset, size)
}

func copyRangeGeneric(src, dst File, srcOffset, dstOffset, size int64) error {
	oldSrcOffset, err := src.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil
	}
	oldDstOffset, err := dst.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil
	}

	// Check that the source file has the data in question
	if fi, err := src.Stat(); err != nil {
		return err
	} else if fi.Size() < srcOffset+size {
		return io.ErrUnexpectedEOF
	}

	// Check that the destination file has sufficient space
	if fi, err := dst.Stat(); err != nil {
		return err
	} else if fi.Size() < dstOffset+size {
		if err := dst.Truncate(dstOffset + size); err != nil {
			return err
		}
	}

	if n, err := src.Seek(srcOffset, io.SeekStart); err != nil {
		return err
	} else if n != srcOffset {
		return io.ErrUnexpectedEOF
	}

	if n, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
		return err
	} else if n != dstOffset {
		return io.ErrUnexpectedEOF
	}

	for size > 0 {
		n, err := io.CopyN(dst, src, size)
		if err != nil {
			_, _ = src.Seek(oldSrcOffset, io.SeekStart)
			_, _ = dst.Seek(oldDstOffset, io.SeekStart)
			return err
		}
		size -= n
	}

	// Restore offsets
	if n, err := src.Seek(oldSrcOffset, io.SeekStart); err != nil {
		return err
	} else if n != oldSrcOffset {
		return io.ErrUnexpectedEOF
	}

	if n, err := dst.Seek(oldDstOffset, io.SeekStart); err != nil {
		return err
	} else if n != oldDstOffset {
		return io.ErrUnexpectedEOF
	}

	return nil
}

func getCopyOptimisations() []string {
	opt := os.Getenv("STCOPYOPTIMISATIONS")
	if opt == "" {
		// ioctl first because it's available on early kernels and works on btrfs
		// copy_file_range is only available on linux 4.5+ and works on xfs and btrfs
		// sendfile does not do any block reuse, but works since 2.6+ or so.
		opt = "ioctl,copy_file_range,sendfile"
	}
	return strings.Split(opt, ",")
}
