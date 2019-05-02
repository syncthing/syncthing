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
	registerCopyRangeImplementation(copyRangeImplementation{
		name: "generic",
		impl: copyRangeGeneric,
	})
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

	if oldSrcOffset != srcOffset {
		if n, err := src.Seek(srcOffset, io.SeekStart); err != nil {
			return err
		} else if n != srcOffset {
			return io.ErrUnexpectedEOF
		}
	}

	if oldDstOffset != dstOffset {
		if n, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
			return err
		} else if n != dstOffset {
			return io.ErrUnexpectedEOF
		}
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
