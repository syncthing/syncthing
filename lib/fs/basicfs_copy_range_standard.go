// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io"
)

func init() {
	registerCopyRangeImplementation(CopyRangeMethodStandard, copyRangeStandard)
}

func copyRangeStandard(src, dst File, srcOffset, dstOffset, size int64) error {
	// Check that the destination file has sufficient space
	if fi, err := dst.Stat(); err != nil {
		return err
	} else if fi.Size() < dstOffset+size {
		if err := dst.Truncate(dstOffset + size); err != nil {
			return err
		}
	}

	// Record old offsets, defer seeking back, best effort.
	oldDstOffset, err := dst.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	defer func() { _, _ = dst.Seek(oldDstOffset, io.SeekStart) }()

	oldSrcOffset, err := src.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	defer func() { _, _ = src.Seek(oldSrcOffset, io.SeekStart) }()

	// Seek to target offsets.
	if srcOffset != oldSrcOffset {
		if _, err := src.Seek(srcOffset, io.SeekStart); err != nil {
			return err
		}
	}
	if dstOffset != oldDstOffset {
		if _, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
			return err
		}
	}

	if _, err = io.CopyN(dst, src, size); err == io.EOF {
		return io.ErrUnexpectedEOF
	} else {
		return err
	}
}
