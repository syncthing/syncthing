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
	"testing"
)

type copyRangeTestScenario struct {
	name        string
	copyFn      copyFuncGeneric
	mustSucceed bool
}

type copyFuncGeneric func(src, dst File, srcOffset, dstOffset, size int64) error
type copyFuncOptimised func(src, dst basicFile, srcOffset, dstOffset, size int64) error

func wrapOptimised(fn copyFuncOptimised) copyFuncGeneric {
	return func(src, dst File, srcOffset, dstOffset, size int64) error {
		srcFile, srcOk := src.(basicFile)
		dstFile, dstOk := dst.(basicFile)
		if srcOk && dstOk {
			return fn(srcFile, dstFile, srcOffset, dstOffset, size)
		}
		fmt.Printf("%#v", src)
		panic("unexpected types")
	}
}

var (
	generationSize = 4 << 20
	copySize       = 1 << 20

	testCases = []struct {
		// Offset from which to read
		srcOffset int
		dstOffset int
		// Cursor position before the copy
		srcPos int
		dstPos int
		// Expected cursor position after the copy
		expectedDstPos  int
		expectedDstSize int
	}{
		{
			srcOffset:       0,
			dstOffset:       generationSize,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstPos:  generationSize + copySize,
			expectedDstSize: generationSize + copySize,
		},
		{
			srcOffset:       0,
			dstOffset:       generationSize,
			srcPos:          0, // We seek back to start, and expect src not to move after copy
			dstPos:          0, // Seek back, but expect dst pos to change
			expectedDstPos:  generationSize + copySize,
			expectedDstSize: generationSize + copySize,
		},
		{
			srcOffset:       copySize,
			dstOffset:       generationSize,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstPos:  generationSize + copySize,
			expectedDstSize: generationSize + copySize,
		},
		{
			srcOffset:       0,
			dstOffset:       0,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstPos:  copySize,
			expectedDstSize: generationSize,
		},
		{
			srcOffset:       copySize,
			dstOffset:       0,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstPos:  copySize,
			expectedDstSize: generationSize,
		},
		// Write way past the end of the file
		{
			srcOffset:       0,
			dstOffset:       generationSize * 2,
			srcPos:          generationSize,
			dstPos:          generationSize,
			expectedDstPos:  generationSize*2 + copySize,
			expectedDstSize: generationSize*2 + copySize,
		},
	}
)

func TestCopyRange(ttt *testing.T) {
	srcBuf := make([]byte, generationSize)
	dstBuf := make([]byte, generationSize*3)
	randSrc := rand.New(rand.NewSource(rand.Int63()))
	for _, testFunc := range copyRangeTests {
		ttt.Run(testFunc.name, func(tt *testing.T) {
			for _, testCase := range testCases {
				name := fmt.Sprintf("%d_%d_%d_%d_%d_%d",
					testCase.srcOffset/copySize,
					testCase.dstOffset/copySize,
					testCase.srcPos/copySize,
					testCase.dstPos/copySize,
					testCase.expectedDstPos/copySize,
					testCase.expectedDstSize/copySize,
				)
				tt.Run(name, func(t *testing.T) {
					td, err := ioutil.TempDir(os.Getenv("STFSTESTOATH"), "")
					if err != nil {
						t.Fatal(err)
					}
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

					if err := writeFull(src, srcBuf); err != nil {
						t.Fatal(err)
					}

					if err := writeFull(dst, dstBuf[:generationSize]); err != nil {
						t.Fatal(err)
					}

					// Set the offsets

					if _, err := src.Seek(int64(testCase.srcPos), io.SeekStart); err != nil {
						t.Fatal(err)
					}

					if _, err := dst.Seek(int64(testCase.dstPos), io.SeekStart); err != nil {
						t.Fatal(err)
					}

					// Copy the data

					if err := testFunc.copyFn(src, dst, int64(testCase.srcOffset), int64(testCase.dstOffset), int64(copySize)); err != nil {
						if testFunc.mustSucceed {
							t.Fatal(err)
						} else {
							// It might be an unsupported syscall, skip.
							t.Skip(err.Error())
						}
					}

					// Check offsets where we expect them

					if srcCurPos, err := src.Seek(0, io.SeekCurrent); err != nil {
						t.Fatal(err)
					} else if srcCurPos != int64(testCase.srcPos) {
						t.Errorf("src pos expected %d got %d", testCase.srcPos, srcCurPos)
					}

					if dstCurPos, err := dst.Seek(0, io.SeekCurrent); err != nil {
						t.Fatal(err)
					} else if dstCurPos != int64(testCase.expectedDstPos) {
						t.Errorf("dst pos expected %d got %d", testCase.expectedDstPos, dstCurPos)
					}

					// Check the data is as expected

					if _, err := dst.Seek(0, io.SeekStart); err != nil {
						t.Fatal(err)
					}

					dstBuf = dstBuf[:testCase.expectedDstSize]

					if _, err := io.ReadFull(dst, dstBuf); err != nil {
						t.Fatal(err)
					}

					if !bytes.Equal(srcBuf[testCase.srcOffset:testCase.srcOffset+copySize], dstBuf[testCase.dstOffset:testCase.dstOffset+copySize]) {
						t.Errorf("Not equal")
					}

					// Check dst size

					if fi, err := dst.Stat(); err != nil {
						t.Fatal(err)
					} else if fi.Size() != int64(testCase.expectedDstSize) {
						t.Errorf("expected %d size, got %d", testCase.expectedDstSize, fi.Size())
					}
				})
			}
		})
	}
}

func writeFull(w io.Writer, buf []byte) error {
	for len(buf) > 0 {
		m, err := w.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[m:]
	}
	return nil
}
