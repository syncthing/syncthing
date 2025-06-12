// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

var (
	generationSize  int64 = 4 << 20
	defaultCopySize int64 = 1 << 20

	testCases = []struct {
		name string
		// Starting size of files
		srcSize int64
		dstSize int64
		// Offset from which to read
		srcOffset int64
		dstOffset int64
		// Cursor position before the copy
		srcStartingPos int64
		dstStartingPos int64
		// Expected destination size
		expectedDstSizeAfterCopy int64
		// Custom copy size
		copySize int64
		// Expected failure
		expectedErrors map[CopyRangeMethod]error
	}{
		{
			name:                     "append to end",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                generationSize,
			srcStartingPos:           generationSize,
			dstStartingPos:           generationSize,
			expectedDstSizeAfterCopy: generationSize + defaultCopySize,
			copySize:                 defaultCopySize,
			expectedErrors:           nil,
		},
		{
			name:                     "append to end offsets at start",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                generationSize,
			srcStartingPos:           0, // We seek back to start, and expect src not to move after copy
			dstStartingPos:           0, // Seek back, but expect dst pos to not change
			expectedDstSizeAfterCopy: generationSize + defaultCopySize,
			copySize:                 defaultCopySize,
			expectedErrors:           nil,
		},
		{
			name:                     "overwrite part of destination region",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                defaultCopySize,
			dstOffset:                generationSize,
			srcStartingPos:           generationSize,
			dstStartingPos:           generationSize,
			expectedDstSizeAfterCopy: generationSize + defaultCopySize,
			copySize:                 defaultCopySize,
			expectedErrors:           nil,
		},
		{
			name:                     "overwrite all of destination",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                0,
			srcStartingPos:           generationSize,
			dstStartingPos:           generationSize,
			expectedDstSizeAfterCopy: generationSize,
			copySize:                 defaultCopySize,
			expectedErrors:           nil,
		},
		{
			name:                     "overwrite part of destination",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                defaultCopySize,
			dstOffset:                0,
			srcStartingPos:           generationSize,
			dstStartingPos:           generationSize,
			expectedDstSizeAfterCopy: generationSize,
			copySize:                 defaultCopySize,
			expectedErrors:           nil,
		},
		// Write way past the end of the file
		{
			name:                     "destination gets expanded as it is being written to",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                generationSize * 2,
			srcStartingPos:           generationSize,
			dstStartingPos:           generationSize,
			expectedDstSizeAfterCopy: generationSize*2 + defaultCopySize,
			copySize:                 defaultCopySize,
			expectedErrors:           nil,
		},
		// Source file does not have enough bytes to copy in that range, should result in an unexpected eof.
		{
			name:                     "source file too small",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: -11, // Does not matter, should fail.
			copySize:                 defaultCopySize * 10,
			// ioctl returns syscall.EINVAL, rest are wrapped
			expectedErrors: map[CopyRangeMethod]error{
				CopyRangeMethodIoctl:            io.ErrUnexpectedEOF,
				CopyRangeMethodStandard:         io.ErrUnexpectedEOF,
				CopyRangeMethodCopyFileRange:    io.ErrUnexpectedEOF,
				CopyRangeMethodSendFile:         io.ErrUnexpectedEOF,
				CopyRangeMethodAllWithFallback:  io.ErrUnexpectedEOF,
				CopyRangeMethodDuplicateExtents: io.ErrUnexpectedEOF,
			},
		},
		{
			name:                     "unaligned source offset unaligned size",
			srcSize:                  generationSize,
			dstSize:                  0,
			srcOffset:                1,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: defaultCopySize + 1,
			copySize:                 defaultCopySize + 1,
			expectedErrors: map[CopyRangeMethod]error{
				CopyRangeMethodIoctl:            syscall.EINVAL,
				CopyRangeMethodDuplicateExtents: syscall.EINVAL,
			},
		},
		{
			name:                     "unaligned source offset aligned size",
			srcSize:                  generationSize,
			dstSize:                  0,
			srcOffset:                1,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: defaultCopySize,
			copySize:                 defaultCopySize,
			expectedErrors: map[CopyRangeMethod]error{
				CopyRangeMethodIoctl:            syscall.EINVAL,
				CopyRangeMethodDuplicateExtents: syscall.EINVAL,
			},
		},
		{
			name:                     "unaligned destination offset unaligned size",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                1,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: generationSize,
			copySize:                 defaultCopySize + 1,
			expectedErrors: map[CopyRangeMethod]error{
				CopyRangeMethodIoctl:            syscall.EINVAL,
				CopyRangeMethodDuplicateExtents: syscall.EINVAL,
			},
		},
		{
			name:                     "unaligned destination offset aligned size",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                1,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: generationSize,
			copySize:                 defaultCopySize,
			expectedErrors: map[CopyRangeMethod]error{
				CopyRangeMethodIoctl:            syscall.EINVAL,
				CopyRangeMethodDuplicateExtents: syscall.EINVAL,
			},
		},
		{
			name:                     "aligned offsets unaligned size",
			srcSize:                  generationSize,
			dstSize:                  generationSize,
			srcOffset:                0,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: generationSize,
			copySize:                 defaultCopySize + 1,
			expectedErrors: map[CopyRangeMethod]error{
				CopyRangeMethodIoctl:            syscall.EINVAL,
				CopyRangeMethodDuplicateExtents: syscall.EINVAL,
			},
		},
		// Last block that starts on a nice boundary
		{
			name:                     "last block",
			srcSize:                  generationSize + 2,
			dstSize:                  0,
			srcOffset:                generationSize,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: 2,
			copySize:                 2,
			// Succeeds on all, as long as the offset is file-system block aligned.
			expectedErrors: nil,
		},
		// Copy whole file
		{
			name:                     "whole file copy block aligned",
			srcSize:                  generationSize,
			dstSize:                  0,
			srcOffset:                0,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: generationSize,
			copySize:                 generationSize,
			expectedErrors:           nil,
		},
		{
			name:                     "whole file copy not block aligned",
			srcSize:                  generationSize + 1,
			dstSize:                  0,
			srcOffset:                0,
			dstOffset:                0,
			srcStartingPos:           0,
			dstStartingPos:           0,
			expectedDstSizeAfterCopy: generationSize + 1,
			copySize:                 generationSize + 1,
			expectedErrors:           nil,
		},
	}
)

func TestCopyRange(tttt *testing.T) {
	randSrc := rand.New(rand.NewSource(rand.Int63()))
	paths := filepath.SplitList(os.Getenv("STFSTESTPATH"))
	if len(paths) == 0 {
		paths = []string{""}
	}
	for _, path := range paths {
		testPath, err := os.MkdirTemp(path, "")
		if err != nil {
			tttt.Fatal(err)
		}
		defer os.RemoveAll(testPath)
		name := path
		if name == "" {
			name = "tmp"
		}
		tttt.Run(name, func(ttt *testing.T) {
			for copyMethod, impl := range copyRangeMethods {
				ttt.Run(copyMethod.String(), func(tt *testing.T) {
					for _, testCase := range testCases {
						tt.Run(testCase.name, func(t *testing.T) {
							srcBuf := make([]byte, testCase.srcSize)
							dstBuf := make([]byte, testCase.dstSize)
							td, err := os.MkdirTemp(testPath, "")
							if err != nil {
								t.Fatal(err)
							}
							defer os.RemoveAll(td)

							fs := NewFilesystem(FilesystemTypeBasic, td)

							if _, err := io.ReadFull(randSrc, srcBuf); err != nil {
								t.Fatal(err)
							}

							if _, err := io.ReadFull(randSrc, dstBuf); err != nil {
								t.Fatal(err)
							}

							src, err := fs.Create("src")
							if err != nil {
								t.Fatal(err)
							}
							defer func() { _ = src.Close() }()

							dst, err := fs.Create("dst")
							if err != nil {
								t.Fatal(err)
							}
							defer func() { _ = dst.Close() }()

							// Write some data

							if _, err := src.Write(srcBuf); err != nil {
								t.Fatal(err)
							}

							if _, err := dst.Write(dstBuf); err != nil {
								t.Fatal(err)
							}

							// Set the offsets

							if n, err := src.Seek(testCase.srcStartingPos, io.SeekStart); err != nil || n != testCase.srcStartingPos {
								t.Fatal(err)
							}

							if n, err := dst.Seek(testCase.dstStartingPos, io.SeekStart); err != nil || n != testCase.dstStartingPos {
								t.Fatal(err)
							}

							srcBasic, ok := unwrap(src).(basicFile)
							if !ok {
								t.Fatal("src file is not a basic file")
							}
							dstBasic, ok := unwrap(dst).(basicFile)
							if !ok {
								t.Fatal("dst file is not a basic file")
							}
							if err := impl(srcBasic, dstBasic, testCase.srcOffset, testCase.dstOffset, testCase.copySize); err != nil {
								if errors.Is(err, errors.ErrUnsupported) {
									// Test runner can adjust directory in which to run the tests, that allow broader tests.
									t.Skip("Not supported on the current filesystem, set STFSTESTPATH env var.")
								}
								if testCase.expectedErrors[copyMethod] == err {
									return
								}
								t.Fatal(err)
							} else if testCase.expectedErrors[copyMethod] != nil {
								t.Fatal("did not get expected error")
							}

							// Check offsets where we expect them

							if srcCurPos, err := src.Seek(0, io.SeekCurrent); err != nil {
								t.Fatal(err)
							} else if srcCurPos != testCase.srcStartingPos {
								t.Errorf("src pos expected %d got %d", testCase.srcStartingPos, srcCurPos)
							}

							if dstCurPos, err := dst.Seek(0, io.SeekCurrent); err != nil {
								t.Fatal(err)
							} else if dstCurPos != testCase.dstStartingPos {
								t.Errorf("dst pos expected %d got %d", testCase.dstStartingPos, dstCurPos)
							}

							// Check dst size

							if fi, err := dst.Stat(); err != nil {
								t.Fatal(err)
							} else if fi.Size() != testCase.expectedDstSizeAfterCopy {
								t.Errorf("expected %d size, got %d", testCase.expectedDstSizeAfterCopy, fi.Size())
							}

							// Check the data is as expected

							if _, err := dst.Seek(0, io.SeekStart); err != nil {
								t.Fatal(err)
							}

							resultBuf := make([]byte, testCase.expectedDstSizeAfterCopy)
							if _, err := io.ReadFull(dst, resultBuf); err != nil {
								t.Fatal(err)
							}

							if !bytes.Equal(srcBuf[testCase.srcOffset:testCase.srcOffset+testCase.copySize], resultBuf[testCase.dstOffset:testCase.dstOffset+testCase.copySize]) {
								t.Errorf("Not equal")
							}

							// Check not copied content does not get corrupted

							if testCase.dstOffset > testCase.dstSize {
								if !bytes.Equal(dstBuf[:testCase.dstSize], resultBuf[:testCase.dstSize]) {
									t.Error("region before copy region not equals")
								}
								if !bytes.Equal(resultBuf[testCase.dstSize:testCase.dstOffset], make([]byte, testCase.dstOffset-testCase.dstSize)) {
									t.Error("found non zeroes in expected zero region")
								}
							} else {
								if !bytes.Equal(dstBuf[:testCase.dstOffset], resultBuf[:testCase.dstOffset]) {
									t.Error("region before copy region not equals")
								}
								afterCopyStart := testCase.dstOffset + testCase.copySize

								if afterCopyStart < testCase.dstSize {
									if !bytes.Equal(dstBuf[afterCopyStart:], resultBuf[afterCopyStart:len(dstBuf)]) {
										t.Error("region after copy region not equals")
									}
								}
							}
						})
					}
				})
			}
		})
	}
}
