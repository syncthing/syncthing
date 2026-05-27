// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"testing"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestBlocks(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal()
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	files := []protocol.FileInfo{
		{
			Name: "file1",
			Blocks: []protocol.BlockInfo{
				{Hash: []byte{1, 2, 3}, Offset: 0, Size: 42},
				{Hash: []byte{2, 3, 4}, Offset: 42, Size: 42},
				{Hash: []byte{3, 4, 5}, Offset: 84, Size: 42},
			},
		},
		{
			Name: "file2",
			Blocks: []protocol.BlockInfo{
				{Hash: []byte{2, 3, 4}, Offset: 0, Size: 42},
				{Hash: []byte{3, 4, 5}, Offset: 42, Size: 42},
				{Hash: []byte{4, 5, 6}, Offset: 84, Size: 42},
			},
		},
	}

	if err := db.Update("test", protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Search for blocks

	vals, err := itererr.Collect(db.AllLocalBlocksWithHash(folderID, []byte{1, 2, 3}))
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 {
		t.Log(vals)
		t.Fatal("expected one hit")
	} else if vals[0].BlockIndex != 0 || vals[0].Offset != 0 || vals[0].Size != 42 {
		t.Log(vals[0])
		t.Fatal("bad entry")
	}
	if vals[0].FileName != "file1" {
		t.Fatal("should be file1")
	}

	// Get the other blocks

	vals, err = itererr.Collect(db.AllLocalBlocksWithHash(folderID, []byte{3, 4, 5}))
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 {
		t.Log(vals)
		t.Fatal("expected two hits")
	}
	// if vals[0].Index != 2 || vals[0].Offset != 84 || vals[0].Size != 42 {
	// 	t.Log(vals[0])
	// 	t.Fatal("bad entry 1")
	// }
	// if vals[1].Index != 1 || vals[1].Offset != 42 || vals[1].Size != 42 {
	// 	t.Log(vals[1])
	// 	t.Fatal("bad entry 2")
	// }
}

func TestBlocksDeleted(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal()
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Insert a file
	file := genFile("foo", 1, 0)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}

	// We should find one entry for the block hash
	search := file.Blocks[0].Hash
	es, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, search))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 {
		t.Fatal("expected one hit")
	}

	// Update the file with a new block hash
	file.Blocks = genBlocks("foo", 42, 1)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}

	// Searching for the old hash should yield no hits
	if hits, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, search)); err != nil {
		t.Fatal(err)
	} else if len(hits) != 0 {
		t.Log(hits)
		t.Error("expected no hits")
	}

	// Searching for the new hash should yield one hits
	if hits, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, file.Blocks[0].Hash)); err != nil {
		t.Fatal(err)
	} else if len(hits) != 1 {
		t.Log(hits)
		t.Error("expected one hit")
	}
}

func TestDropBlockIndex(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdb.Close() })

	// Insert files with blocks
	files := []protocol.FileInfo{
		genFile("a", 3, 0),
		genFile("b", 2, 0),
	}
	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Verify blocks exist
	hits, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, files[0].Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected block hits before drop")
	}

	// Drop the block index
	if err := sdb.DropBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}

	// Verify blocks are gone
	hits, err = itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, files[0].Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatal("expected no block hits after drop")
	}

	// Dropping again should be a no-op (already empty)
	if err := sdb.DropBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}

	// Dropping a nonexistent folder should be fine
	if err := sdb.DropBlockIndex("nonexistent"); err != nil {
		t.Fatal(err)
	}
}

func TestPopulateBlockIndex(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdb.Close() })

	// Insert files with blocks
	files := []protocol.FileInfo{
		genFile("a", 3, 0),
		genFile("b", 2, 0),
	}
	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Collect the original block entries for comparison
	origHitsA, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, files[0].Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(origHitsA) != 1 {
		t.Fatal("expected one hit for block a[0]")
	}

	// Drop the block index
	if err := sdb.DropBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}

	// Populate it back from existing blocklists
	if err := sdb.PopulateBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}

	// Verify all blocks are back
	for i, f := range files {
		for j, b := range f.Blocks {
			hits, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, b.Hash))
			if err != nil {
				t.Fatal(err)
			}
			if len(hits) == 0 {
				t.Errorf("file %d block %d: expected hits after populate", i, j)
			}
		}
	}

	// Populating again should be a no-op (not empty)
	if err := sdb.PopulateBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}
}

func TestPopulateBlockIndexSkipsRemoteFiles(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdb.Close() })

	// Insert a local file (blocks indexed) and a remote file (blocks not indexed)
	localFile := genFile("local", 2, 0)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{localFile}); err != nil {
		t.Fatal(err)
	}
	remoteFile := genFile("remote", 2, 1)
	if err := sdb.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{remoteFile}); err != nil {
		t.Fatal(err)
	}

	// Drop and repopulate
	if err := sdb.DropBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}
	if err := sdb.PopulateBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}

	// Local file blocks should be present
	hits, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, localFile.Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Error("expected hits for local file blocks")
	}

	// Remote file blocks should not be present (blocks are only
	// indexed for local files)
	hits, err = itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, remoteFile.Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Error("expected no hits for remote file blocks")
	}
}

func TestSkipBlockIndexOnUpdate(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdb.Close() })

	// Insert a file with SkipBlockIndex
	file := genFile("a", 3, 0)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}, db.WithSkipBlockIndex()); err != nil {
		t.Fatal(err)
	}

	// Blocks should not be indexed
	hits, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, file.Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatal("expected no block hits with SkipBlockIndex")
	}

	// The blocklist should still be stored (file info is retrievable with blocks)
	fi, ok, err := sdb.GetDeviceFile(folderID, protocol.LocalDeviceID, "a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("file not found")
	}
	if len(fi.Blocks) != 3 {
		t.Fatalf("expected 3 blocks in file info, got %d", len(fi.Blocks))
	}

	// Populate should fill in the blocks
	if err := sdb.PopulateBlockIndex(folderID); err != nil {
		t.Fatal(err)
	}

	hits, err = itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, file.Blocks[0].Hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatal("expected one hit after populate")
	}
}

func TestRemoteSequence(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal()
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Insert a local file
	file := genFile("foo", 1, 0)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}

	// Insert several remote files
	file = genFile("foo1", 1, 42)
	if err := sdb.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}
	if err := sdb.Update(folderID, protocol.DeviceID{43}, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}
	file = genFile("foo2", 1, 43)
	if err := sdb.Update(folderID, protocol.DeviceID{43}, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}
	if err := sdb.Update(folderID, protocol.DeviceID{44}, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}
	file = genFile("foo3", 1, 44)
	if err := sdb.Update(folderID, protocol.DeviceID{44}, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}

	// Verify remote sequences
	seqs, err := sdb.RemoteSequences(folderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 3 || seqs[protocol.DeviceID{42}] != 42 ||
		seqs[protocol.DeviceID{43}] != 43 ||
		seqs[protocol.DeviceID{44}] != 44 {
		t.Log(seqs)
		t.Error("bad seqs")
	}
}
