// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package weakhash

import (
	"bytes"
	"context"
	"fmt"
	"hash"
	vadler32 "hash/adler32"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/chmduquesne/rollinghash/adler32"
	"github.com/chmduquesne/rollinghash/bozo32"
	"github.com/chmduquesne/rollinghash/buzhash32"
	"github.com/chmduquesne/rollinghash/buzhash64"
)

const (
	testFile = "../model/testdata/tmpfile"
	size     = 128 << 10
)

func BenchmarkFind1MFile(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		fd, err := os.Open(testFile)
		if err != nil {
			b.Fatal(err)
		}
		_, err = Find(context.Background(), fd, []uint32{0, 1, 2}, size)
		if err != nil {
			b.Fatal(err)
		}
		fd.Close()
	}
}

type RollingHash interface {
	hash.Hash
	Roll(byte)
}

func BenchmarkBlock(b *testing.B) {
	tests := []struct {
		name string
		hash hash.Hash
	}{
		{
			"adler32", adler32.New(),
		},
		{
			"bozo32", bozo32.New(),
		},
		{
			"buzhash32", buzhash32.New(),
		},
		{
			"buzhash64", buzhash64.New(),
		},
		{
			"vanilla-adler32", vadler32.New(),
		},
	}

	sizes := []int64{128 << 10, 16 << 20}

	buf := make([]byte, 16<<20)
	rand.Read(buf)

	for _, testSize := range sizes {
		for _, test := range tests {
			b.Run(test.name+"-"+fmt.Sprint(testSize), func(bb *testing.B) {
				bb.Run("", func(bbb *testing.B) {
					bbb.ResetTimer()
					for i := 0; i < bbb.N; i++ {
						lr := io.LimitReader(bytes.NewReader(buf), testSize)
						n, err := io.Copy(test.hash, lr)
						if err != nil {
							bbb.Error(err)
						}
						if n != testSize {
							bbb.Errorf("%d != %d", n, testSize)
						}

						test.hash.Sum(nil)
						test.hash.Reset()
					}

					bbb.SetBytes(testSize)
					bbb.ReportAllocs()
				})
			})
		}
	}
}

func BenchmarkRoll(b *testing.B) {
	tests := []struct {
		name string
		hash RollingHash
	}{
		{
			"adler32", adler32.New(),
		},
		{
			"bozo32", bozo32.New(),
		},
		{
			"buzhash32", buzhash32.New(),
		},
		{
			"buzhash64", buzhash64.New(),
		},
	}

	sizes := []int64{128 << 10, 16 << 20}

	for _, testSize := range sizes {
		for _, test := range tests {
			b.Run(test.name+"-"+fmt.Sprint(testSize), func(bb *testing.B) {
				bb.Run("", func(bbb *testing.B) {
					data := make([]byte, testSize)

					if _, err := test.hash.Write(data); err != nil {
						bbb.Error(err)
					}

					bbb.ResetTimer()

					for i := 0; i < bbb.N; i++ {
						for j := int64(0); j <= testSize; j++ {
							test.hash.Roll('a')
						}
					}

					bbb.SetBytes(testSize)
					bbb.ReportAllocs()
				})
			})
		}
	}
}
