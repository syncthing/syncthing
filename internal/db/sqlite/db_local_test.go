package sqlite

import (
	"testing"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestBlocks(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
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

	vals, err := itererr.Collect(db.AllLocalBlocksWithHash([]byte{1, 2, 3}))
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

	// Get FileInfos for those blocks

	found := 0
	it, errFn := db.AllLocalFilesWithBlocksHashAnyFolder(vals[0].BlocklistHash)
	for folder, fileInfo := range it {
		if folder != folderID {
			t.Fatal("should be same folder")
		}
		if fileInfo.Name != "file1" {
			t.Fatal("should be file1")
		}
		found++
	}
	if err := errFn(); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatal("should find one file")
	}

	// Get the other blocks

	vals, err = itererr.Collect(db.AllLocalBlocksWithHash([]byte{3, 4, 5}))
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

	sdb, err := OpenTemp()
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
	es := mustCollect[db.BlockMapEntry](t)(sdb.AllLocalBlocksWithHash(search))
	if len(es) != 1 {
		t.Fatal("expected one hit")
	}

	// Update the file with a new block hash
	file.Blocks = genBlocks("foo", 42, 1)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}); err != nil {
		t.Fatal()
	}

	// Searching for the old hash should yield no hits
	if hits := mustCollect[db.BlockMapEntry](t)(sdb.AllLocalBlocksWithHash(search)); len(hits) != 0 {
		t.Log(hits)
		t.Error("expected no hits")
	}

	// Searching for the new hash should yield one hits
	if hits := mustCollect[db.BlockMapEntry](t)(sdb.AllLocalBlocksWithHash(file.Blocks[0].Hash)); len(hits) != 1 {
		t.Log(hits)
		t.Error("expected one hit")
	}
}
