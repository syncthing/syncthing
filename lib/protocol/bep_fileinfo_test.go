// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"crypto/sha256"
	"testing"

	"github.com/syncthing/syncthing/lib/build"
)

func TestLocalFlagBits(t *testing.T) {
	var f FileInfo
	if f.IsIgnored() || f.MustRescan() || f.IsInvalid() {
		t.Error("file should have no weird bits set by default")
	}

	f.SetIgnored()
	if !f.IsIgnored() || f.MustRescan() || !f.IsInvalid() {
		t.Error("file should be ignored and invalid")
	}

	f.SetMustRescan()
	if f.IsIgnored() || !f.MustRescan() || !f.IsInvalid() {
		t.Error("file should be must-rescan and invalid")
	}

	f.SetUnsupported()
	if f.IsIgnored() || f.MustRescan() || !f.IsInvalid() {
		t.Error("file should be invalid")
	}
}

func TestIsEquivalent(t *testing.T) {
	b := func(v bool) *bool {
		return &v
	}

	type testCase struct {
		a         FileInfo
		b         FileInfo
		ignPerms  *bool // nil means should not matter, we'll test both variants
		ignBlocks *bool
		ignFlags  FlagLocal
		eq        bool
	}
	cases := []testCase{
		// Empty FileInfos are equivalent
		{eq: true},

		// Various basic attributes, all of which cause inequality when
		// they differ
		{
			a:  FileInfo{Name: "foo"},
			b:  FileInfo{Name: "bar"},
			eq: false,
		},
		{
			a:  FileInfo{Type: FileInfoTypeFile},
			b:  FileInfo{Type: FileInfoTypeDirectory},
			eq: false,
		},
		{
			a:  FileInfo{Size: 1234},
			b:  FileInfo{Size: 2345},
			eq: false,
		},
		{
			a:  FileInfo{Deleted: false},
			b:  FileInfo{Deleted: true},
			eq: false,
		},
		{
			a:  FileInfo{LocalFlags: 0},
			b:  FileInfo{LocalFlags: FlagLocalRemoteInvalid},
			eq: false,
		},
		{
			a:  FileInfo{ModifiedS: 1234},
			b:  FileInfo{ModifiedS: 2345},
			eq: false,
		},
		{
			a:  FileInfo{ModifiedNs: 1234},
			b:  FileInfo{ModifiedNs: 2345},
			eq: false,
		},

		// Special handling of local flags and invalidity. "MustRescan"
		// files are never equivalent to each other. Otherwise, equivalence
		// is based just on whether the file becomes IsInvalid() or not, not
		// the specific reason or flag bits.
		{
			a:  FileInfo{LocalFlags: FlagLocalMustRescan},
			b:  FileInfo{LocalFlags: FlagLocalMustRescan},
			eq: false,
		},
		{
			a:  FileInfo{LocalFlags: FlagLocalRemoteInvalid},
			b:  FileInfo{LocalFlags: FlagLocalRemoteInvalid},
			eq: true,
		},
		{
			a:  FileInfo{LocalFlags: FlagLocalUnsupported},
			b:  FileInfo{LocalFlags: FlagLocalUnsupported},
			eq: true,
		},
		{
			a:  FileInfo{LocalFlags: FlagLocalRemoteInvalid},
			b:  FileInfo{LocalFlags: FlagLocalUnsupported},
			eq: true,
		},
		{
			a:  FileInfo{LocalFlags: 0},
			b:  FileInfo{LocalFlags: FlagLocalReceiveOnly},
			eq: false,
		},
		{
			a:        FileInfo{LocalFlags: 0},
			b:        FileInfo{LocalFlags: FlagLocalReceiveOnly},
			ignFlags: FlagLocalReceiveOnly,
			eq:       true,
		},

		// Difference in blocks is not OK
		{
			a:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{1, 2, 3, 4}}}},
			b:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{2, 3, 4, 5}}}},
			ignBlocks: b(false),
			eq:        false,
		},

		// ... unless we say it is
		{
			a:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{1, 2, 3, 4}}}},
			b:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{2, 3, 4, 5}}}},
			ignBlocks: b(true),
			eq:        true,
		},

		// Difference in permissions is not OK.
		{
			a:        FileInfo{Permissions: 0o444},
			b:        FileInfo{Permissions: 0o666},
			ignPerms: b(false),
			eq:       false,
		},

		// ... unless we say it is
		{
			a:        FileInfo{Permissions: 0o666},
			b:        FileInfo{Permissions: 0o444},
			ignPerms: b(true),
			eq:       true,
		},

		// These attributes are not checked at all
		{
			a:  FileInfo{NoPermissions: false},
			b:  FileInfo{NoPermissions: true},
			eq: true,
		},
		{
			a:  FileInfo{Version: Vector{Counters: []Counter{{ID: 1, Value: 42}}}},
			b:  FileInfo{Version: Vector{Counters: []Counter{{ID: 42, Value: 1}}}},
			eq: true,
		},
		{
			a:  FileInfo{Sequence: 1},
			b:  FileInfo{Sequence: 2},
			eq: true,
		},

		// The block size is not checked (but this would fail the blocks
		// check in real world)
		{
			a:  FileInfo{RawBlockSize: 1},
			b:  FileInfo{RawBlockSize: 2},
			eq: true,
		},

		// The symlink target is checked for symlinks
		{
			a:  FileInfo{Type: FileInfoTypeSymlink, SymlinkTarget: []byte("a")},
			b:  FileInfo{Type: FileInfoTypeSymlink, SymlinkTarget: []byte("b")},
			eq: false,
		},

		// ... but not for non-symlinks
		{
			a:  FileInfo{Type: FileInfoTypeFile, SymlinkTarget: []byte("a")},
			b:  FileInfo{Type: FileInfoTypeFile, SymlinkTarget: []byte("b")},
			eq: true,
		},
		// Unix Ownership should be the same
		{
			a:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			b:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			eq: true,
		},
		// ... but matching ID is enough
		{
			a:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			b:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "B", GroupName: "B", UID: 1000, GID: 1000}}},
			eq: true,
		},
		// ... or matching name
		{
			a:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			b:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1001, GID: 1001}}},
			eq: true,
		},
		// ... or empty name
		{
			a:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			b:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "", GroupName: "", UID: 1000, GID: 1000}}},
			eq: true,
		},
		// ... but not different ownership
		{
			a:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			b:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "B", GroupName: "B", UID: 1001, GID: 1001}}},
			eq: false,
		},
		// or missing ownership
		{
			a:  FileInfo{Platform: PlatformData{Unix: &UnixData{OwnerName: "A", GroupName: "A", UID: 1000, GID: 1000}}},
			b:  FileInfo{Platform: PlatformData{}},
			eq: false,
		},
	}

	if build.IsWindows {
		// On windows we only check the user writable bit of the permission
		// set, so these are equivalent.
		cases = append(cases, testCase{
			a:        FileInfo{Permissions: 0o777},
			b:        FileInfo{Permissions: 0o600},
			ignPerms: b(false),
			eq:       true,
		})
	}

	for i, tc := range cases {
		// Check the standard attributes with all permutations of the
		// special ignore flags, unless the value of those flags are given
		// in the tests.
		for _, ignPerms := range []bool{true, false} {
			for _, ignBlocks := range []bool{true, false} {
				if tc.ignPerms != nil && *tc.ignPerms != ignPerms {
					continue
				}
				if tc.ignBlocks != nil && *tc.ignBlocks != ignBlocks {
					continue
				}

				if res := tc.a.isEquivalent(tc.b, FileInfoComparison{IgnorePerms: ignPerms, IgnoreBlocks: ignBlocks, IgnoreFlags: tc.ignFlags}); res != tc.eq {
					t.Errorf("Case %d:\na: %v\nb: %v\na.IsEquivalent(b, %v, %v) => %v, expected %v", i, tc.a, tc.b, ignPerms, ignBlocks, res, tc.eq)
				}
				if res := tc.b.isEquivalent(tc.a, FileInfoComparison{IgnorePerms: ignPerms, IgnoreBlocks: ignBlocks, IgnoreFlags: tc.ignFlags}); res != tc.eq {
					t.Errorf("Case %d:\na: %v\nb: %v\nb.IsEquivalent(a, %v, %v) => %v, expected %v", i, tc.a, tc.b, ignPerms, ignBlocks, res, tc.eq)
				}
			}
		}
	}
}

func TestSha256OfEmptyBlock(t *testing.T) {
	// every block size should have a correct entry in sha256OfEmptyBlock
	for blockSize := MinBlockSize; blockSize <= MaxBlockSize; blockSize *= 2 {
		expected := sha256.Sum256(make([]byte, blockSize))
		if sha256OfEmptyBlock[blockSize] != expected {
			t.Error("missing or wrong hash for block of size", blockSize)
		}
	}
}

func TestBlocksEqual(t *testing.T) {
	blocksOne := []BlockInfo{{Hash: []byte{1, 2, 3, 4}}}
	blocksTwo := []BlockInfo{{Hash: []byte{5, 6, 7, 8}}}
	hashOne := []byte{42, 42, 42, 42}
	hashTwo := []byte{29, 29, 29, 29}

	cases := []struct {
		b1 []BlockInfo
		h1 []byte
		b2 []BlockInfo
		h2 []byte
		eq bool
	}{
		{blocksOne, hashOne, blocksOne, hashOne, true},  // everything equal
		{blocksOne, hashOne, blocksTwo, hashTwo, false}, // nothing equal
		{blocksOne, hashOne, blocksOne, nil, true},      // blocks compared
		{blocksOne, nil, blocksOne, nil, true},          // blocks compared
		{blocksOne, nil, blocksTwo, nil, false},         // blocks compared
		{blocksOne, hashOne, blocksTwo, hashOne, true},  // hashes equal, blocks not looked at
		{blocksOne, hashOne, blocksOne, hashTwo, true},  // hashes different, blocks compared
		{blocksOne, hashOne, blocksTwo, hashTwo, false}, // hashes different, blocks compared
		{blocksOne, hashOne, nil, nil, false},           // blocks is different from no blocks
		{blocksOne, nil, nil, nil, false},               // blocks is different from no blocks
		{nil, hashOne, nil, nil, true},                  // nil blocks are equal, even of one side has a hash
	}

	for _, tc := range cases {
		f1 := FileInfo{Blocks: tc.b1, BlocksHash: tc.h1}
		f2 := FileInfo{Blocks: tc.b2, BlocksHash: tc.h2}

		if !f1.BlocksEqual(f1) {
			t.Error("f1 is always equal to itself", f1)
		}
		if !f2.BlocksEqual(f2) {
			t.Error("f2 is always equal to itself", f2)
		}
		if res := f1.BlocksEqual(f2); res != tc.eq {
			t.Log("f1", f1.BlocksHash, f1.Blocks)
			t.Log("f2", f2.BlocksHash, f2.Blocks)
			t.Errorf("f1.BlocksEqual(f2) == %v but should be %v", res, tc.eq)
		}
		if res := f2.BlocksEqual(f1); res != tc.eq {
			t.Log("f1", f1.BlocksHash, f1.Blocks)
			t.Log("f2", f2.BlocksHash, f2.Blocks)
			t.Errorf("f2.BlocksEqual(f1) == %v but should be %v", res, tc.eq)
		}
	}
}
