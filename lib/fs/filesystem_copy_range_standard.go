// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"io"
)

func init() {
	registerCopyRangeImplementation(CopyRangeMethodStandard, copyRangeStandard)
}

func copyRangeStandard(src, dst File, srcOffset, dstOffset, size int64) error {
	const bufSize = 4 << 20

	buf := make([]byte, bufSize)

	// TODO: In go 1.15, we should use file.ReadFrom that uses copy_file_range underneath.

	// ReadAt and WriteAt does not modify the position of the file.
	for size > 0 {
		if size < bufSize {
			buf = buf[:size]
		}
		n, err := src.ReadAt(buf, srcOffset)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}
		if _, err = dst.WriteAt(buf[:n], dstOffset); err != nil {
			return err
		}
		srcOffset += int64(n)
		dstOffset += int64(n)
		size -= int64(n)
	}

	return nil
}
