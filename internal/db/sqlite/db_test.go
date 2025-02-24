package sqlite

import (
	"iter"
	"path/filepath"
	"sync"
	"testing"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestBasics(t *testing.T) {
	t.Parallel()

	blocklistIndirectCutoff = 0

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
		{Name: "test1", Size: 1, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Type: protocol.FileInfoTypeDirectory, Version: v, Blocks: genBlocks(2)},
		{Name: "test2/a", Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2/b", Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test3", Sequence: 101, Size: 100, ModifiedS: 300, Version: v.Update(42), Blocks: genBlocks(1)},
		{Name: "test4", Sequence: 102, Size: 200, ModifiedS: 100, Version: v.Update(42), Blocks: genBlocks(2)},
		{Name: "test1", Sequence: 103, Size: 300, ModifiedS: 200, Version: v.Update(42), Blocks: genBlocks(3)},
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
		if len(fi.Blocks) != 2 {
			t.Fatal("expected two blocks")
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

		fi, ok, err := db.Global(folderID, "test1")
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
		if len(have) != 4 {
			t.Log(have)
			t.Error("expected four files")
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
		if len(need) != 3 || need[0] != "test1" {
			t.Log(need)
			t.Error("expected three files, ordered alphabetically")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 1))
		if len(need) != 1 || need[0] != "test1" {
			t.Log(need)
			t.Error("expected one file, limited, ordered alphabetically")
		}

		need = iterCollectTest(t, db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderLargestFirst, 0))
		if len(need) != 3 || need[0] != "test1" { // largest
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

	t.Run("LocalSize", func(t *testing.T) {
		t.Parallel()

		// Local device

		c := db.LocalSize(folderID, protocol.LocalDeviceID)
		if c.Files != 3 {
			t.Log(c)
			t.Error("one file expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != 1+128+100+200 {
			t.Log(c)
			t.Error("size 1+128+100+200 expected")
		}

		// Other device

		c = db.LocalSize(folderID, protocol.DeviceID{42})
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

		c := db.GlobalSize(folderID)
		if c.Files != 5 {
			t.Log(c)
			t.Error("five files expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != 128+100+200+100+200+300 {
			t.Log(c)
			t.Error("size 128+100+200+100+200+300 expected")
		}
	})

	t.Run("NeedSizeLocal", func(t *testing.T) {
		t.Parallel()

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

		c := db.NeedSize(folderID, protocol.DeviceID{42})
		if c.Files != 2 {
			t.Log(c)
			t.Error("two files expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != 128+100+200 {
			t.Log(c)
			t.Error("size 128 expected")
		}
	})

	t.Run("Folders", func(t *testing.T) {
		t.Parallel()

		folders, err := db.Folders()
		if err != nil {
			t.Fatal(err)
		}
		if len(folders) != 1 || folders[0] != folderID {
			t.Error("expected one folder")
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

	t.Run("Sequence", func(t *testing.T) {
		t.Parallel()

		if seq, err := db.Sequence(folderID, protocol.LocalDeviceID); err != nil {
			t.Fatal(err)
		} else if seq != 4 {
			t.Log(seq)
			t.Error("expected local sequence to match number of files inserted")
		}

		if seq, err := db.Sequence(folderID, protocol.DeviceID{42}); err != nil {
			t.Fatal(err)
		} else if seq != 103 {
			t.Log(seq)
			t.Error("expected remote sequence to match highest sent")
		}

		// Non-existent should be zero and no error
		if seq, err := db.Sequence("trolol", protocol.LocalDeviceID); err != nil {
			t.Fatal(err)
		} else if seq != 0 {
			t.Log(seq)
			t.Error("expected zero sequence")
		}
		if seq, err := db.Sequence("trolol", protocol.DeviceID{42}); err != nil {
			t.Fatal(err)
		} else if seq != 0 {
			t.Log(seq)
			t.Error("expected zero sequence")
		}
		if seq, err := db.Sequence(folderID, protocol.DeviceID{99}); err != nil {
			t.Fatal(err)
		} else if seq != 0 {
			t.Log(seq)
			t.Error("expected zero sequence")
		}
	})

	t.Run("AllGlobalPrefix", func(t *testing.T) {
		t.Parallel()

		vals := iterCollectTest(t, db.AllGlobalPrefix(folderID, "test2"))

		// Vals should be test2, test2/a, test2/b
		if len(vals) != 3 {
			t.Log(vals)
			t.Error("expected three items")
		} else if vals[0].Name != "test2" {
			t.Error(vals)
		}

		// Empty prefix should be all the files
		vals = iterCollectTest(t, db.AllGlobalPrefix(folderID, ""))

		if len(vals) != 6 {
			t.Log(vals)
			t.Error("expected six items")
		}
	})

	t.Run("AllLocalPrefix", func(t *testing.T) {
		t.Parallel()

		vals := iterCollectTest(t, db.AllLocalPrefixed(folderID, protocol.LocalDeviceID, "test2"))

		// Vals should be test2, test2/a, test2/b
		if len(vals) != 3 {
			t.Log(vals)
			t.Error("expected three items")
		} else if vals[0].Name != "test2" {
			t.Error(vals)
		}

		// Empty prefix should be all the files
		vals = iterCollectTest(t, db.AllLocalPrefixed(folderID, protocol.LocalDeviceID, ""))

		if len(vals) != 4 {
			t.Log(vals)
			t.Error("expected four items")
		}
	})

	t.Run("AllLocalSequenced", func(t *testing.T) {
		t.Parallel()

		vals := iterCollectTest(t, db.AllLocalSequenced(folderID, protocol.LocalDeviceID, 3))

		// Vals should be test2/a, test2/b
		if len(vals) != 2 {
			t.Log(vals)
			t.Error("expected three items")
		} else if vals[0].Name != "test2/a" || vals[0].Sequence != 3 {
			t.Error(vals)
		}
	})
}

func TestAvailability(t *testing.T) {
	db, err := OpenMemory()
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

func TestDropFilesNamed(t *testing.T) {
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
		{Name: "test1", Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop test1
	if err := db.DropFilesNamed(folderID, protocol.LocalDeviceID, []string{"test1"}); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.Local(folderID, protocol.LocalDeviceID, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c := db.LocalSize(folderID, protocol.LocalDeviceID); c.Files != 1 {
		t.Log(c)
		t.Error("expected count to be one")
	}
	if _, ok, err := db.Local(folderID, protocol.LocalDeviceID, "test2"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
}

func TestDropFolder(t *testing.T) {
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

	// Folder A
	err = db.Update("a", protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test1", Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Folder B
	err = db.Update("b", protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test1", Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop A
	if err := db.DropFolder("a"); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.Local("a", protocol.LocalDeviceID, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c := db.LocalSize("a", protocol.LocalDeviceID); c.Files != 0 {
		t.Log(c)
		t.Error("expected count to be zero")
	}

	if _, ok, err := db.Local("b", protocol.LocalDeviceID, "test1"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
	if c := db.LocalSize("b", protocol.LocalDeviceID); c.Files != 2 {
		t.Log(c)
		t.Error("expected count to be two")
	}
}

func TestDropDevice(t *testing.T) {
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

	// Device 1
	err = db.Update("a", protocol.DeviceID{1}, []protocol.FileInfo{
		{Name: "test1", Sequence: 1, Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Sequence: 2, Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Device 2
	err = db.Update("a", protocol.DeviceID{2}, []protocol.FileInfo{
		{Name: "test1", Sequence: 1, Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Sequence: 2, Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop 1
	if err := db.DropDevice(protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.Local("a", protocol.DeviceID{1}, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c := db.LocalSize("a", protocol.DeviceID{1}); c.Files != 0 {
		t.Log(c)
		t.Error("expected count to be zero")
	}
	if _, ok, err := db.Local("a", protocol.DeviceID{2}, "test1"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
	if c := db.LocalSize("a", protocol.DeviceID{2}); c.Files != 2 {
		t.Log(c)
		t.Error("expected count to be two")
	}

	// Drop something that doesn't exist
	if err := db.DropDevice(protocol.DeviceID{99}); err != nil {
		t.Fatal(err)
	}
}

func TestDropAllFiles(t *testing.T) {
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

	// Device 1 folder A
	err = db.Update("a", protocol.DeviceID{1}, []protocol.FileInfo{
		{Name: "test1", Sequence: 1, Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Sequence: 2, Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Device 1 folder B
	err = db.Update("b", protocol.DeviceID{1}, []protocol.FileInfo{
		{Name: "test1", Sequence: 1, Size: 100, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Sequence: 2, Size: 200, Version: v, Blocks: genBlocks(2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop folder A
	if err := db.DropAllFiles("a", protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.Local("a", protocol.DeviceID{1}, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c := db.LocalSize("a", protocol.DeviceID{1}); c.Files != 0 {
		t.Log(c)
		t.Error("expected count to be zero")
	}
	if _, ok, err := db.Local("b", protocol.DeviceID{1}, "test1"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
	if c := db.LocalSize("b", protocol.DeviceID{1}); c.Files != 2 {
		t.Log(c)
		t.Error("expected count to be two")
	}

	// Drop things that don't exist
	if err := db.DropAllFiles("a", protocol.DeviceID{99}); err != nil {
		t.Fatal(err)
	}
	if err := db.DropAllFiles("trolol", protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	}
	if err := db.DropAllFiles("trolol", protocol.DeviceID{99}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentUpdate(t *testing.T) {
	t.Parallel()

	db, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	const folderID = "test"

	var v protocol.Vector
	v = v.Update(1)
	files := []protocol.FileInfo{
		{Name: "test1", Sequence: 100, Size: 1, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Sequence: 101, Type: protocol.FileInfoTypeDirectory, Size: 128, Version: v, Blocks: genBlocks(2)},
		{Name: "test3", Sequence: 102, Size: 100, ModifiedS: 300, Version: v.Update(42), Blocks: genBlocks(1)},
		{Name: "test4", Sequence: 103, Size: 200, ModifiedS: 100, Version: v.Update(42), Blocks: genBlocks(2)},
	}

	const n = 32
	res := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			res[i] = db.Update(folderID, protocol.DeviceID{byte(i), byte(i), byte(i)}, files)
			wg.Done()
		}()
	}
	wg.Wait()
	for i, err := range res {
		if err != nil {
			t.Errorf("%d: %v", i, err)
		}
	}
}

func TestConcurrentUpdateSelect(t *testing.T) {
	t.Parallel()

	db, err := Open(filepath.Join(t.TempDir(), "db"))
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
	files := []protocol.FileInfo{
		{Name: "test1", Sequence: 1, Size: 1, Version: v, Blocks: genBlocks(2)},
		{Name: "test2", Sequence: 2, Type: protocol.FileInfoTypeDirectory, Size: 128, Version: v, Blocks: genBlocks(2)},
		{Name: "test3", Sequence: 3, Size: 100, ModifiedS: 300, Version: v.Update(42), Blocks: genBlocks(1)},
		{Name: "test4", Sequence: 4, Size: 200, ModifiedS: 100, Version: v.Update(42), Blocks: genBlocks(2)},
	}

	// Insert the files for a remote device
	if err := db.Update(folderID, protocol.DeviceID{42}, files); err != nil {
		t.Fatal()
	}

	// Iterate over handled files and insert them for the local device.
	// This is similar to a pattern we have in other places and should
	// work.
	handled := 0
	for name, err := range db.AllNeededNames(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0) {
		if err != nil {
			t.Fatal(err)
		}

		glob, ok, err := db.Global(folderID, name)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("should exist")
		}

		glob.Version = glob.Version.Update(1)
		if err := db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{glob}); err != nil {
			t.Fatal(err)
		}

		handled++
	}

	if handled != len(files) {
		t.Log(handled)
		t.Error("should have handled all the files")
	}
}

func TestAllForBlocksHash(t *testing.T) {
	t.Parallel()

	db, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	const folderID = "test"

	var v protocol.Vector
	v = v.Update(1)

	// test1 is unique, while test2 and test3 have the same blocks and hence
	// the same blocks hash

	files := []protocol.FileInfo{
		{Name: "test1", Sequence: 1, Size: 100, Version: v, Blocks: genBlocks(1)},
		{Name: "test2", Sequence: 2, Size: 200, Version: v, Blocks: genBlocks(2)},
		{Name: "test3", Sequence: 3, Size: 300, Version: v, Blocks: genBlocks(2)},
	}

	if err := db.Update(folderID, protocol.DeviceID{42}, files); err != nil {
		t.Fatal(err)
	}

	// Check test1

	test1, ok, err := db.Local(folderID, protocol.DeviceID{42}, "test1")
	if err != nil || !ok {
		t.Fatal("expected to exist")
	}

	vals := iterCollectTest(t, db.AllForBlocksHash(folderID, test1.BlocksHash))
	if len(vals) != 1 {
		t.Log(vals)
		t.Fatal("expected one file to match")
	}

	// Check test2 which also matches test3

	test2, ok, err := db.Local(folderID, protocol.DeviceID{42}, "test2")
	if err != nil || !ok {
		t.Fatal("expected to exist")
	}

	vals = iterCollectTest(t, db.AllForBlocksHash(folderID, test2.BlocksHash))
	if len(vals) != 2 {
		t.Log(vals)
		t.Fatal("expected two files to match")
	}
	if vals[0].Name != "test2" {
		t.Log(vals[0])
		t.Error("expected test2")
	}
	if vals[1].Name != "test3" {
		t.Log(vals[1])
		t.Error("expected test3")
	}
}

func iterCollectTest[T any](t *testing.T, it iter.Seq2[T, error]) []T {
	t.Helper()
	vals, err := itererr.Collect(it)
	if err != nil {
		t.Fatal(err)
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
