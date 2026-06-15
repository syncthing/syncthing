// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Tests for resumable temp-file reuse on receive-encrypted folders. Shared
// helpers used by these and the stress tests (in the _stress_test.go file) live
// here.

package model

import (
	"bytes"
	"slices"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

const encBlockSize = 131072 // 128 KiB, matching the block-size fixtures.

// realFSFolder returns a sendReceiveFolder backed by a real temp-dir filesystem.
// Sparse/SEEK_HOLE detection needs a real filesystem, not the in-memory fake, so
// the test is skipped where the platform/filesystem cannot detect holes.
func realFSFolder(t *testing.T) *sendReceiveFolder {
	t.Helper()
	_, f := setupSendReceiveFolder(t)
	f.mtimefs = fs.NewFilesystem(fs.FilesystemTypeBasic, t.TempDir())
	if !holeDetectionSupported(t, f) {
		t.Skip("sparse hole detection unavailable on this platform/filesystem")
	}
	return f
}

// holeDetectionSupported reports whether f's filesystem can detect sparse holes.
func holeDetectionSupported(t *testing.T, f *sendReceiveFolder) bool {
	t.Helper()
	name := fs.TempName("holeprobe")
	fd, err := f.mtimefs.Create(name)
	must(t, err)
	fs.SetSparse(fd)
	must(t, fd.Truncate(1<<20))
	_, ok := fs.NextHole(fd, 0)
	fd.Close()
	f.mtimefs.Remove(name)
	return ok
}

// buildEncFile makes a FileInfo of n full 128 KiB blocks plus an optional
// partial tail block of tailSize bytes (0 = none). Hashes are unique per block
// so nothing collides.
func buildEncFile(name string, n, tailSize int) protocol.FileInfo {
	var blks []protocol.BlockInfo
	var off int64
	for i := 0; i < n; i++ {
		h := make([]byte, 32)
		h[0] = byte(i + 1)
		h[1] = name[len(name)-1]
		blks = append(blks, protocol.BlockInfo{Offset: off, Size: encBlockSize, Hash: h})
		off += encBlockSize
	}
	if tailSize > 0 {
		blks = append(blks, protocol.BlockInfo{Offset: off, Size: tailSize, Hash: []byte{0xee}})
		off += int64(tailSize)
	}
	return protocol.FileInfo{Name: name, Blocks: blks, Size: off}
}

// writeSparseTemp creates a full-size sparse temp for file and writes (allocates)
// exactly the blocks whose index is true in present, returning the temp name.
func writeSparseTemp(t *testing.T, f *sendReceiveFolder, file protocol.FileInfo, present []bool) string {
	t.Helper()
	tempName := fs.TempName(file.Name)
	fd, err := f.mtimefs.Create(tempName)
	must(t, err)
	fs.SetSparse(fd)
	must(t, fd.Truncate(file.Size))
	for i, blk := range file.Blocks {
		if present[i] && blk.Size > 0 {
			if _, err := fd.WriteAt(bytes.Repeat([]byte{0xab}, int(blk.Size)), blk.Offset); err != nil {
				t.Fatal(err)
			}
		}
	}
	must(t, fd.Sync())
	fd.Close()
	return tempName
}

// runReuse runs reuseBlocksEncrypted on file's temp, returning the blocks still
// to download and the indices reused from the temp.
func runReuse(f *sendReceiveFolder, file protocol.FileInfo, tempName string) ([]protocol.BlockInfo, []int) {
	blocks := append([]protocol.BlockInfo{}, file.Blocks...)
	reused := make([]int, 0, len(file.Blocks))
	return f.reuseBlocksEncrypted(blocks, reused, file, tempName)
}

// wantReuse returns the indices reuseBlocksEncrypted should reuse and the block
// offsets it should re-fetch, given which blocks are present in the temp. When
// no block is a hole nothing is reused (the no-hole fail-safe) and every block
// is re-fetched.
func wantReuse(file protocol.FileInfo, present []bool) (reused []int, download []int64) {
	anyHole := slices.Contains(present, false)
	for i, p := range present {
		if anyHole && p {
			reused = append(reused, i)
		} else {
			download = append(download, file.Blocks[i].Offset)
		}
	}
	return reused, download
}

// downloadOffsets is the list of offsets in a download block list.
func downloadOffsets(blocks []protocol.BlockInfo) []int64 {
	offs := make([]int64, len(blocks))
	for i, b := range blocks {
		offs[i] = b.Offset
	}
	return offs
}

func allTrue(n int) []bool {
	s := make([]bool, n)
	for i := range s {
		s[i] = true
	}
	return s
}

// A partially-downloaded encrypted temp (some blocks written, some sparse holes)
// must be reused: written blocks are kept, holes are re-fetched, and the temp is
// not deleted.
func TestReuseBlocksEncryptedSparse(t *testing.T) {
	f := realFSFolder(t)
	file := buildEncFile("encfile", 8, 0)
	present := []bool{true, true, true, true, false, false, false, false}
	tempName := writeSparseTemp(t, f, file, present)

	download, reused := runReuse(f, file, tempName)

	wantReused, wantDL := wantReuse(file, present)
	if !slices.Equal(reused, wantReused) {
		t.Errorf("reused = %v, want %v", reused, wantReused)
	}
	if got := downloadOffsets(download); !slices.Equal(got, wantDL) {
		t.Errorf("download offsets = %v, want %v", got, wantDL)
	}
	if _, err := f.mtimefs.Stat(tempName); err != nil {
		t.Errorf("temp file must NOT be deleted on reuse: %v", err)
	}
}

// With no temp file present, nothing is reused and all blocks are downloaded.
func TestReuseBlocksEncryptedNoTemp(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	file := buildEncFile("encfile2", 3, 0)

	download, reused := runReuse(f, file, fs.TempName(file.Name))

	if len(reused) != 0 || len(download) != 3 {
		t.Errorf("no-temp: reused=%v download=%d, want reused=[] download=3", reused, len(download))
	}
}

// Reuse must never cross files: fileB (no temp of its own) must not reuse
// fileA's partially-written temp. reuseBlocksEncrypted only opens this file's
// own temp, but assert it so a regression can't leak data between files.
func TestReuseBlocksEncryptedNoCrossFile(t *testing.T) {
	f := realFSFolder(t)
	fileA := buildEncFile("fileA", 8, 0)
	fileB := buildEncFile("fileB", 8, 0) // identical layout, different name
	writeSparseTemp(t, f, fileA, []bool{true, true, true, true, false, false, false, false})

	download, reused := runReuse(f, fileB, fs.TempName(fileB.Name))

	if len(reused) != 0 {
		t.Errorf("CROSS-FILE REUSE: fileB reused %d blocks from fileA's temp (must be 0)", len(reused))
	}
	if len(download) != len(fileB.Blocks) {
		t.Errorf("fileB download = %d blocks, want all %d", len(download), len(fileB.Blocks))
	}
}

// A temp with no holes is refused (see reuseBlocksEncrypted): it can't be told
// apart from a half-written temp on a filesystem without hole support. Writing
// every block fully allocates the temp, doubling as the non-sparse fallback.
func TestReuseBlocksEncryptedNoHoles(t *testing.T) {
	f := realFSFolder(t)
	file := buildEncFile("allfile", 8, 0)
	tempName := writeSparseTemp(t, f, file, allTrue(len(file.Blocks)))

	download, reused := runReuse(f, file, tempName)

	if len(reused) != 0 {
		t.Errorf("reused %d blocks, want 0 (no hole found -> refuse reuse)", len(reused))
	}
	if len(download) != len(file.Blocks) {
		t.Errorf("download = %d blocks, want all %d (re-fetch everything)", len(download), len(file.Blocks))
	}
}

// A temp whose size does not match the file (a leftover from a different, here
// one-block-longer, version under the same name) must not be reused, since
// encrypted blocks can't be content-verified.
func TestReuseBlocksEncryptedWrongSize(t *testing.T) {
	f := realFSFolder(t)
	file := buildEncFile("verfile", 8, 0)
	bigger := buildEncFile("verfile", 9, 0) // same name => same temp path, larger size
	tempName := writeSparseTemp(t, f, bigger, []bool{true, true, true, true, false, false, false, false, false})

	download, reused := runReuse(f, file, tempName)

	if len(reused) != 0 {
		t.Errorf("size-mismatched temp must not be reused, reused=%v", reused)
	}
	if len(download) != len(file.Blocks) {
		t.Errorf("download = %d, want all %d", len(download), len(file.Blocks))
	}
}
