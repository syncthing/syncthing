package sqlitedb

import (
	"iter"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestGetAndHave(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "TestHave.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	const folderID = "test"

	// Some local files
	var v protocol.Vector
	v = v.Update(1)
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test", Size: 1, Version: v},
		{Name: "test2", Size: 1, Version: v},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test3", Sequence: 1, Size: 42, Version: v.Update(42)},
		{Name: "test4", Sequence: 2, Size: 42, Version: v.Update(42)},
		{Name: "test", Sequence: 3, Size: 42, Version: v.Update(42)},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Get", func(t *testing.T) {
		fi, ok, err := db.Get(folderID, protocol.LocalDeviceID, "test2") // exists
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("not found")
		}
		if fi.Name != "test2" {
			t.Fatal("should have got test2")
		}

		_, ok, err = db.Get(folderID, protocol.LocalDeviceID, "test3") // does not exist
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("should be not found")
		}
	})

	t.Run("GetGlobal", func(t *testing.T) {
		fi, ok, err := db.GetGlobal(folderID, "test")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("not found")
		}
		if fi.Size != 42 {
			t.Fatal("should be the remote file")
		}
	})

	t.Run("Have", func(t *testing.T) {
		have := collectIter(t, db.Have(folderID, protocol.LocalDeviceID))
		if len(have) != 2 {
			t.Log(have)
			t.Error("expected two files")
		}
		have = collectIter(t, db.Have(folderID, protocol.DeviceID{42}))
		if len(have) != 3 {
			t.Log(have)
			t.Error("expected three files")
		}
	})

	t.Run("Need", func(t *testing.T) {
		need := collectIter(t, db.Need(folderID, protocol.LocalDeviceID))
		if len(need) != 3 {
			t.Log(need)
			t.Error("expected three files")
		}
		need = collectIter(t, db.Need(folderID, protocol.DeviceID{42}))
		if len(need) != 0 {
			t.Log(need)
			t.Error("expected no files")
		}
	})

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func collectIter[T any](t *testing.T, it iter.Seq2[T, error]) []T {
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
