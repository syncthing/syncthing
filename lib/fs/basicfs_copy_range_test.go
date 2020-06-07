// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"syscall"
	"testing"
)

var (
	generationSize  int64 = 4 << 20
	defaultCopySize int64 = 1 << 20

	testCases = []struct {
		// Offset from which to read
		srcOffset int64
		dstOffset int64
		// Cursor position before the copy
		srcPos int64
		dstPos int64
		// Expected destination size
		expectedDstSize int64
		// Custom copy size
		copySize int64
		// Expected failure
		expectedErrors []error
	}{
		{
			srcOffset:       0,
			dstOffset:       generationSize,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstSize: generationSize + defaultCopySize,
			copySize:        defaultCopySize,
			expectedErrors:  nil,
		},
		{
			srcOffset:       0,
			dstOffset:       generationSize,
			srcPos:          0, // We seek back to start, and expect src not to move after copy
			dstPos:          0, // Seek back, but expect dst pos to not change
			expectedDstSize: generationSize + defaultCopySize,
			copySize:        defaultCopySize,
			expectedErrors:  nil,
		},
		{
			srcOffset:       defaultCopySize,
			dstOffset:       generationSize,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstSize: generationSize + defaultCopySize,
			copySize:        defaultCopySize,
			expectedErrors:  nil,
		},
		{
			srcOffset:       0,
			dstOffset:       0,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstSize: generationSize,
			copySize:        defaultCopySize,
			expectedErrors:  nil,
		},
		{
			srcOffset:       defaultCopySize,
			dstOffset:       0,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstSize: generationSize,
			copySize:        defaultCopySize,
			expectedErrors:  nil,
		},
		// Write way past the end of the file
		{
			srcOffset:       0,
			dstOffset:       generationSize * 2,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstSize: generationSize*2 + defaultCopySize,
			copySize:        defaultCopySize,
			expectedErrors:  nil,
		},
		// Source file does not have enough bytes to copy in that range, should result in an unexpected eof.
		{
			srcOffset:       0,
			dstOffset:       0,
			srcPos:          0,
			dstPos:          0,
			expectedDstSize: -11, // Does not matter, should fail.
			copySize:        defaultCopySize * 10,
			// ioctl returns syscall.EINVAL, rest are wrapped
			expectedErrors: []error{io.ErrUnexpectedEOF, syscall.EINVAL},
		},
	}
)

func TestCopyRange(ttt *testing.T) {
	srcBuf := make([]byte, generationSize)
	dstBuf := make([]byte, generationSize*3)
	randSrc := rand.New(rand.NewSource(rand.Int63()))
	for copyType, impl := range copyRangeImplementations {
		ttt.Run(copyType.String(), func(tt *testing.T) {
			for _, testCase := range testCases {
				name := fmt.Sprintf("%d_%d_%d_%d_%d_%d_%t",
					testCase.srcOffset/defaultCopySize,
					testCase.dstOffset/defaultCopySize,
					testCase.srcPos/defaultCopySize,
					testCase.dstPos/defaultCopySize,
					testCase.expectedDstSize/defaultCopySize,
					testCase.copySize/defaultCopySize,
					testCase.expectedErrors == nil,
				)
				tt.Run(name, func(t *testing.T) {
					td, err := ioutil.TempDir(os.Getenv("STFSTESTPATH"), "")
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = os.RemoveAll(td) }()
					fs := NewFilesystem(FilesystemTypeBasic, td)

					if _, err := io.ReadFull(randSrc, srcBuf); err != nil {
						t.Fatal(err)
					}

					if _, err := io.ReadFull(randSrc, dstBuf[:generationSize]); err != nil {
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

					if _, err := dst.Write(dstBuf[:generationSize]); err != nil {
						t.Fatal(err)
					}

					// Set the offsets

					if n, err := src.Seek(testCase.srcPos, io.SeekStart); err != nil || n != testCase.srcPos {
						t.Fatal(err)
					}

					if n, err := dst.Seek(testCase.dstPos, io.SeekStart); err != nil || n != testCase.dstPos {
						t.Fatal(err)
					}

					if err := impl(src.(basicFile), dst.(basicFile), testCase.srcOffset, testCase.dstOffset, testCase.copySize); err != nil {
						if err == syscall.ENOTSUP {
							// Test runner can adjust directory in which to run the tests, that allow broader tests.
							t.Skip("Not supported on the current filesystem, set STFSTESTPATH env var.")
						}
						for _, expectedErr := range testCase.expectedErrors {
							if expectedErr == err {
								return
							}
						}
						t.Fatal(err)
					} else if testCase.expectedErrors != nil {
						t.Fatal("did not get expected error")
					}

					// Check offsets where we expect them

					if srcCurPos, err := src.Seek(0, io.SeekCurrent); err != nil {
						t.Fatal(err)
					} else if srcCurPos != testCase.srcPos {
						t.Errorf("src pos expected %d got %d", testCase.srcPos, srcCurPos)
					}

					if dstCurPos, err := dst.Seek(0, io.SeekCurrent); err != nil {
						t.Fatal(err)
					} else if dstCurPos != testCase.dstPos {
						t.Errorf("dst pos expected %d got %d", testCase.dstPos, dstCurPos)
					}

					// Check the data is as expected

					if _, err := dst.Seek(0, io.SeekStart); err != nil {
						t.Fatal(err)
					}

					dstBuf = dstBuf[:testCase.expectedDstSize]

					if _, err := io.ReadFull(dst, dstBuf); err != nil {
						t.Fatal(err)
					}

					if !bytes.Equal(srcBuf[testCase.srcOffset:testCase.srcOffset+testCase.copySize], dstBuf[testCase.dstOffset:testCase.dstOffset+testCase.copySize]) {
						t.Errorf("Not equal")
					}

					// Check dst size

					if fi, err := dst.Stat(); err != nil {
						t.Fatal(err)
					} else if fi.Size() != testCase.expectedDstSize {
						t.Errorf("expected %d size, got %d", testCase.expectedDstSize, fi.Size())
					}
				})
			}
		})
	}
}
