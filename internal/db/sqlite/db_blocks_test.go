package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestBlocks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}

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

	vals, err := itererr.Collect(db.Blocks([]byte{1, 2, 3}))
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 {
		t.Log(vals)
		t.Fatal("expected one hit")
	} else if vals[0].Name != "file1" || vals[0].Index != 0 || vals[0].Offset != 0 || vals[0].Size != 42 {
		t.Log(vals[0])
		t.Fatal("bad entry")
	}

	vals, err = itererr.Collect(db.Blocks([]byte{3, 4, 5}))
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 {
		t.Log(vals)
		t.Fatal("expected two hits")
	}
	if vals[0].FolderID != "test" || vals[0].Name != "file1" || vals[0].Index != 2 || vals[0].Offset != 84 || vals[0].Size != 42 {
		t.Log(vals[0])
		t.Fatal("bad entry 1")
	}
	if vals[1].FolderID != "test" || vals[1].Name != "file2" || vals[1].Index != 1 || vals[1].Offset != 42 || vals[1].Size != 42 {
		t.Log(vals[1])
		t.Fatal("bad entry 2")
	}
}
