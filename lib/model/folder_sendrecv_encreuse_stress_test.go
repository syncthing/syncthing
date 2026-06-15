// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Stress and property tests for encrypted temp-file reuse: randomized presence
// patterns and edge cases. Shared helpers live in the sibling _encreuse_test.go.

package model

import (
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// TestReuseBlocksEncryptedRandomized hammers the reuse logic with random
// presence patterns and asserts reuse matches wantReuse.
func TestReuseBlocksEncryptedRandomized(t *testing.T) {
	f := realFSFolder(t)

	rng := rand.New(rand.NewSource(0xC0DECAFE))
	for iter := 0; iter < 200; iter++ {
		n := 1 + rng.Intn(16)
		tail := 0
		if rng.Intn(2) == 0 {
			tail = 1 + rng.Intn(encBlockSize-1)
		}
		file := buildEncFile("randfile", n, tail)

		present := make([]bool, len(file.Blocks))
		for i := range present {
			present[i] = rng.Intn(2) == 0
		}
		tempName := writeSparseTemp(t, f, file, present)

		download, reused := runReuse(f, file, tempName)
		sort.Ints(reused)

		wantReused, wantDL := wantReuse(file, present)
		if !slices.Equal(reused, wantReused) {
			t.Fatalf("iter %d n=%d tail=%d present=%v: reused=%v want=%v", iter, n, tail, present, reused, wantReused)
		}
		if got := downloadOffsets(download); !slices.Equal(got, wantDL) {
			t.Fatalf("iter %d n=%d tail=%d: download=%v want=%v", iter, n, tail, got, wantDL)
		}

		f.mtimefs.Remove(tempName)
	}
}

// TestReuseBlocksEncryptedEdges covers structural corner cases one at a time.
func TestReuseBlocksEncryptedEdges(t *testing.T) {
	f := realFSFolder(t)

	allFalse := func(n int) []bool { return make([]bool, n) }
	lastHole := func(n int) []bool { b := allTrue(n); b[n-1] = false; return b }

	cases := []struct {
		name    string
		nFull   int
		tail    int
		present func(n int) []bool
	}{
		{"single-block-present", 1, 0, allTrue},
		{"single-block-hole", 1, 0, allFalse},
		{"all-holes", 8, 0, allFalse},
		{"first-block-hole", 4, 0, func(n int) []bool { b := allTrue(n); b[0] = false; return b }},
		{"last-block-hole", 4, 0, lastHole},
		{"alternating", 8, 0, func(n int) []bool {
			b := make([]bool, n)
			for i := range b {
				b[i] = i%2 == 0
			}
			return b
		}},
		{"partial-tail-present", 3, 777, allTrue},
		{"partial-tail-hole", 3, 777, lastHole},
		{"tiny-tail-present", 0, 17, allTrue},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := buildEncFile("edge"+tc.name[:1], tc.nFull, tc.tail)
			present := tc.present(len(file.Blocks))
			tempName := writeSparseTemp(t, f, file, present)

			download, reused := runReuse(f, file, tempName)
			sort.Ints(reused)

			wantReused, _ := wantReuse(file, present)
			if !slices.Equal(reused, wantReused) {
				t.Errorf("%s: reused=%v want=%v", tc.name, reused, wantReused)
			}
			if len(download)+len(reused) != len(file.Blocks) {
				t.Errorf("%s: download(%d)+reused(%d) != total(%d)", tc.name, len(download), len(reused), len(file.Blocks))
			}
			f.mtimefs.Remove(tempName)
		})
	}
}
