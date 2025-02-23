package sqlite

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

const folderID = "test"

func TestDropRecalcsGlobal(t *testing.T) {
	// When we drop a device we may get a new global

	t.Parallel()

	t.Run("DropAllFiles", func(t *testing.T) {
		t.Parallel()

		testDropWithDropper(t, func(t *testing.T, db *DB) {
			t.Helper()
			if err := db.DropAllFiles(folderID, protocol.DeviceID{42}); err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("DropDevice", func(t *testing.T) {
		t.Parallel()

		testDropWithDropper(t, func(t *testing.T, db *DB) {
			t.Helper()
			if err := db.DropDevice(protocol.DeviceID{42}); err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("DropFilesNamed", func(t *testing.T) {
		t.Parallel()

		testDropWithDropper(t, func(t *testing.T, db *DB) {
			t.Helper()
			if err := db.DropFilesNamed(folderID, protocol.DeviceID{42}, []string{"test1", "test42"}); err != nil {
				t.Fatal(err)
			}
		})
	})
}

func testDropWithDropper(t *testing.T, dropper func(t *testing.T, db *DB)) {
	t.Helper()

	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	var v protocol.Vector
	v = v.Update(1)
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test1", Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test1", Sequence: 103, Size: 300, ModifiedS: 200, Version: v.Update(42), Blocks: genBlocks(3)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Remote test1 wins as the global, verify.
	if db.GlobalSize(folderID).Bytes != 200+300 {
		t.Fatal("bad global size to begin with")
	}
	if g, ok, err := db.Global(folderID, "test1"); err != nil || !ok {
		t.Fatal("missing global to begin with")
	} else if g.Size != 300 {
		t.Fatal("remote test1 should be the global")
	}

	// Now remove that remote device
	dropper(t, db)

	// Our test1 should now be the global
	if db.GlobalSize(folderID).Bytes != 100+200 {
		t.Fatal("bad global size after drop")
	}
	if g, ok, err := db.Global(folderID, "test1"); err != nil || !ok {
		t.Fatal("missing global after drop")
	} else if g.Size != 100 {
		t.Fatal("local test1 should be the global")
	}
}

func TestNeedDeleted(t *testing.T) {
	t.Parallel()

	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	var v protocol.Vector
	v = v.Update(1)
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test1", Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A remote deleted file
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test1", Sequence: 103, Deleted: true, ModifiedS: 200, Version: v.Update(42)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// We need the one deleted file
	s := db.NeedSize(folderID, protocol.LocalDeviceID)
	if s.Bytes != 0 || s.Deleted != 1 {
		t.Log(s)
		t.Error("bad need")
	}
}
