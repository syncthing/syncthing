// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLongTempFilename(t *testing.T) {
	filename := strings.Repeat("l", 300)
	tFile := TempName(filename)
	if len(tFile) < 10 || len(tFile) > 160 {
		t.Fatal("Invalid long filename")
	}
	if !strings.HasSuffix(TempName("short"), "short.tmp") {
		t.Fatal("Invalid short filename", TempName("short"))
	}
}

func benchmarkTempName(b *testing.B, filename string) {
	filename = filepath.Join("/Users/marieantoinette", filename)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		TempName(filename)
	}
}

func BenchmarkTempNameShort(b *testing.B) { benchmarkTempName(b, "somefile.txt") }
func BenchmarkTempNameLong(b *testing.B)  { benchmarkTempName(b, strings.Repeat("a", 270)) }
