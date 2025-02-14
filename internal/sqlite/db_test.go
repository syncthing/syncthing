package sqlite

import (
	"iter"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestBasics(t *testing.T) {
	db, err := Open(filepath.Join(".", "basics.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	const folderID = "test"

	// Some local files
	var v protocol.Vector
	v = v.Update(1)
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test", Size: 1, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Type: protocol.FileInfoTypeDirectory, Size: 128, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test3", Sequence: 1, Size: 100, ModifiedS: 300, Version: v.Update(42), Blocks: genBlocks(1)},
		{Name: "test4", Sequence: 2, Size: 200, ModifiedS: 100, Version: v.Update(42), Blocks: genBlocks(2)},
		{Name: "test", Sequence: 3, Size: 300, ModifiedS: 200, Version: v.Update(42), Blocks: genBlocks(3)},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Local", func(t *testing.T) {
		fi, ok, err := db.Local(folderID, protocol.LocalDeviceID, "test2") // exists
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("not found")
		}
		if fi.Name != "test2" {
			t.Fatal("should have got test2")
		}

		_, ok, err = db.Local(folderID, protocol.LocalDeviceID, "test3") // does not exist
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("should be not found")
		}
	})

	t.Run("Global", func(t *testing.T) {
		fi, ok, err := db.Global(folderID, "test")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("not found")
		}
		if fi.Size != 300 {
			t.Fatal("should be the remote file")
		}
	})

	t.Run("AllLocal", func(t *testing.T) {
		have := iterCollectTest(t, db.AllLocal(folderID, protocol.LocalDeviceID))
		if len(have) != 2 {
			t.Log(have)
			t.Error("expected two files")
		}
		have = iterCollectTest(t, db.AllLocal(folderID, protocol.DeviceID{42}))
		if len(have) != 3 {
			t.Log(have)
			t.Error("expected three files")
		}
	})

	t.Run("AllNeededNamesLocal", func(t *testing.T) {
		need := iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic))
		if len(need) != 3 || need[0] != "test" {
			t.Log(need)
			t.Error("expected three files, ordered alphabetically")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderLargestFirst))
		if len(need) != 3 || need[0] != "test" { // largest
			t.Log(need)
			t.Error("expected three files, ordered largest to smallest")
		}
		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderSmallestFirst))
		if len(need) != 3 || need[0] != "test3" { // smallest
			t.Log(need)
			t.Error("expected three files, ordered smallest to largest")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderNewestFirst))
		if len(need) != 3 || need[0] != "test3" { // newest
			t.Log(need)
			t.Error("expected three files, ordered newest to oldest")
		}
		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderOldestFirst))
		if len(need) != 3 || need[0] != "test4" { // oldest
			t.Log(need)
			t.Error("expected three files, ordered oldest to newest")
		}
	})

	t.Run("AllNeededNamesRemote", func(t *testing.T) {
		t.Skip("materialized needs for remote devices not implemented")
		need := iterCollectTest(t, db.AllNeededNames(folderID, protocol.DeviceID{42}, config.PullOrderAlphabetic))
		if len(need) != 1 {
			t.Log(need)
			t.Error("expected one file")
		}
	})

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func iterCollectTest[T any](t *testing.T, it iter.Seq2[T, error]) []T {
	t.Helper()
	var vals []T
	for v, err := range it {
		if err != nil {
			t.Fatal(err)
		}
		vals = append(vals, v)
	}
	return vals
}

func genBlocks(n int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, n)
	for i := range b {
		h := make([]byte, 32)
		for j := range h {
			h[j] = byte(i + j)
		}
		b[i].Hash = h
		b[i].Size = 128 << 10
		b[i].Offset = (128 << 10) * int64(i)
	}
	return b
}
