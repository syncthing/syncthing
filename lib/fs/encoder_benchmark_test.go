// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"testing"
)

func benchmarkSetup(b *testing.B) *BasicFilesystem {
	dir := b.TempDir()
	var opts []Option
	opts = append(opts, new(OptionFatEncoder))
	return newBasicFilesystem(dir, opts...)
}

func BenchmarkEncoderChange(b *testing.B) {
	fs := benchmarkSetup(b)

	var cases = make(encodeTestCases)
	for input, expected := range encodeNameCases[FilesystemEncoderTypeFat] {
		if input != expected {
			cases[input] = expected
		}
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for input := range cases {
			fs.encoder.encode(input)
		}
	}
}

func BenchmarkEncoderMaybeChange(b *testing.B) {
	fs := benchmarkSetup(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for input := range encodeNameCases[FilesystemEncoderTypeFat] {
			fs.encoder.encode(input)
		}
	}
}

func BenchmarkEncoderNoChange(b *testing.B) {
	fs := benchmarkSetup(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, expected := range encodeNameCases[FilesystemEncoderTypeFat] {
			fs.encoder.encode(expected)
		}
	}
}
