// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package weakhash

import (
	"os"
	"testing"

	"github.com/chmduquesne/rollinghash/adler32"
)

const testFile = "../model/testdata/~syncthing~file.tmp"
const size = 128 << 10

func BenchmarkFind1MFile(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		fd, err := os.Open(testFile)
		if err != nil {
			b.Fatal(err)
		}
		_, err = Find(fd, []uint32{0, 1, 2}, size)
		if err != nil {
			b.Fatal(err)
		}
		fd.Close()
	}
}

func BenchmarkWeakHashAdler32(b *testing.B) {
	data := make([]byte, size)
	hf := adler32.New()

	for i := 0; i < b.N; i++ {
		hf.Write(data)
	}

	_ = hf.Sum32()
	b.SetBytes(size)
}

func BenchmarkWeakHashAdler32Roll(b *testing.B) {
	data := make([]byte, size)
	hf := adler32.New()
	hf.Write(data)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := 0; i <= size; i++ {
			hf.Roll('a')
		}
	}

	b.SetBytes(size)
}
