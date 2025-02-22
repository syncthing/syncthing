package sqlite

import (
	"iter"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestBasics(t *testing.T) {
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
		need := iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0))
		if len(need) != 3 || need[0] != "test" {
			t.Log(need)
			t.Error("expected three files, ordered alphabetically")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 1))
		if len(need) != 1 || need[0] != "test" {
			t.Log(need)
			t.Error("expected one file, limited, ordered alphabetically")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderLargestFirst, 0))
		if len(need) != 3 || need[0] != "test" { // largest
			t.Log(need)
			t.Error("expected three files, ordered largest to smallest")
		}
		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderSmallestFirst, 0))
		if len(need) != 3 || need[0] != "test3" { // smallest
			t.Log(need)
			t.Error("expected three files, ordered smallest to largest")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderNewestFirst, 0))
		if len(need) != 3 || need[0] != "test3" { // newest
			t.Log(need)
			t.Error("expected three files, ordered newest to oldest")
		}
		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderOldestFirst, 0))
		if len(need) != 3 || need[0] != "test4" { // oldest
			t.Log(need)
			t.Error("expected three files, ordered oldest to newest")
		}
	})

	t.Run("AllNeededNamesRemote", func(t *testing.T) {
		t.Parallel()
		t.Skip("materialized needs for remote devices not implemented")
		need := iterCollectTest(t, db.AllNeededNames(folderID, protocol.DeviceID{42}, config.PullOrderAlphabetic, 0))
		if len(need) != 1 {
			t.Log(need)
			t.Error("expected one file")
		}
	})

	t.Run("LocalSize", func(t *testing.T) {
		t.Parallel()
		// The local size is the sum of the files a device has
		c := db.LocalSize(folderID, protocol.LocalDeviceID)
		if c.Files != 1 {
			t.Log(c)
			t.Error("one file expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != 1+128 {
			t.Log(c)
			t.Error("size 1+128 expected")
		}
	})

	t.Run("RemoteSize", func(t *testing.T) {
		t.Parallel()
		// The local size is the sum of the files a device has
		c := db.LocalSize(folderID, protocol.DeviceID{42})
		if c.Files != 3 {
			t.Log(c)
			t.Error("three files expected")
		}
		if c.Directories != 0 {
			t.Log(c)
			t.Error("no directories expected")
		}
		if c.Bytes != 600 {
			t.Log(c)
			t.Error("size 600 expected")
		}
	})

	t.Run("GlobalSize", func(t *testing.T) {
		t.Parallel()
		// The global size is the sum of all the latest-version files
		c := db.GlobalSize(folderID)
		if c.Files != 3 {
			t.Log(c)
			t.Error("one file expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != 128+100+200+300 {
			t.Log(c)
			t.Error("size 128+100+200+300 expected")
		}
	})

	t.Run("NeedSizeLocal", func(t *testing.T) {
		t.Parallel()
		// The need size is the sum of all the latest-version files the device does not have
		c := db.NeedSize(folderID, protocol.LocalDeviceID)
		if c.Files != 3 {
			t.Log(c)
			t.Error("one file expected")
		}
		if c.Directories != 0 {
			t.Log(c)
			t.Error("no directories expected")
		}
		if c.Bytes != 100+200+300 {
			t.Log(c)
			t.Error("size 100+200+300 expected")
		}
	})

	t.Run("NeedSizeRemote", func(t *testing.T) {
		t.Parallel()
		// The need size is the sum of all the latest-version files the device does not have
		c := db.NeedSize(folderID, protocol.DeviceID{42})
		if c.Files != 0 {
			t.Log(c)
			t.Error("no files expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != 128 {
			t.Log(c)
			t.Error("size 128 expected")
		}
	})

	t.Run("DevicesForFolder", func(t *testing.T) {
		t.Parallel()
		devs, err := db.DevicesForFolder("test")
		if err != nil {
			t.Fatal(err)
		}
		if len(devs) != 1 || devs[0] != (protocol.DeviceID{42}) {
			t.Log(devs)
			t.Error("expected one device")
		}
	})
}

func TestAvailability(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "basics.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	const folderID = "test"

	// Some local files
	var v protocol.Vector
	v = v.Update(1)
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test1", Size: 100, ModifiedS: 100, Version: v, Blocks: genBlocks(1)},
		{Name: "test2", Size: 200, ModifiedS: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test2", Sequence: 1, Size: 200, ModifiedS: 200, Version: v, Blocks: genBlocks(1)},
		{Name: "test3", Sequence: 2, Size: 300, ModifiedS: 300, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Further remote files
	err = db.Update(folderID, protocol.DeviceID{45}, []protocol.FileInfo{
		{Name: "test3", Sequence: 1, Size: 200, ModifiedS: 200, Version: v, Blocks: genBlocks(1)},
		{Name: "test4", Sequence: 2, Size: 300, ModifiedS: 300, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	a, err := db.Availability(folderID, "test1")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 0 {
		t.Log(a)
		t.Error("expected no availability (only local)")
	}

	a, err = db.Availability(folderID, "test2")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0] != (protocol.DeviceID{42}) {
		t.Log(a)
		t.Error("expected one availability (only 42)")
	}

	a, err = db.Availability(folderID, "test3")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || a[0] != (protocol.DeviceID{42}) || a[1] != (protocol.DeviceID{45}) {
		t.Log(a)
		t.Error("expected two availabilities (both remotes)")
	}

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
