// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"slices"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestNeed(t *testing.T) {
	t.Helper()

	db, err := OpenTemp()
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
	baseV := v.Update(1)
	newerV := baseV.Update(42)
	files := []protocol.FileInfo{
		genFile("test1", 1, 0), // remote need
		genFile("test2", 2, 0), // local need
		genFile("test3", 3, 0), // global
	}
	files[0].Version = baseV
	files[1].Version = baseV
	files[2].Version = newerV
	err = db.Update(folderID, protocol.LocalDeviceID, files)
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	remote := []protocol.FileInfo{
		genFile("test2", 2, 100), // global
		genFile("test3", 3, 101), // remote need
		genFile("test4", 4, 102), // local need
	}
	remote[0].Version = newerV
	remote[1].Version = baseV
	remote[2].Version = newerV
	err = db.Update(folderID, protocol.DeviceID{42}, remote)
	if err != nil {
		t.Fatal(err)
	}

	// A couple are needed locally
	localNeed := fiNames(mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0)))
	if !slices.Equal(localNeed, []string{"test2", "test4"}) {
		t.Log(localNeed)
		t.Fatal("bad local need")
	}

	// Another couple are needed remotely
	remoteNeed := fiNames(mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.DeviceID{42}, config.PullOrderAlphabetic, 0, 0)))
	if !slices.Equal(remoteNeed, []string{"test1", "test3"}) {
		t.Log(remoteNeed)
		t.Fatal("bad remote need")
	}
}

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

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		genFile("test1", 1, 0),
		genFile("test2", 2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	remote := []protocol.FileInfo{
		genFile("test1", 3, 0),
	}
	remote[0].Version = remote[0].Version.Update(42)
	err = db.Update(folderID, protocol.DeviceID{42}, remote)
	if err != nil {
		t.Fatal(err)
	}

	// Remote test1 wins as the global, verify.
	count, err := db.CountGlobal(folderID)
	if err != nil {
		t.Fatal(err)
	}
	if count.Bytes != (2+3)*128<<10 {
		t.Log(count)
		t.Fatal("bad global size to begin with")
	}
	if g, ok, err := db.GetGlobalFile(folderID, "test1"); err != nil || !ok {
		t.Fatal("missing global to begin with")
	} else if g.Size != 3*128<<10 {
		t.Fatal("remote test1 should be the global")
	}

	// Now remove that remote device
	dropper(t, db)

	// Our test1 should now be the global
	count, err = db.CountGlobal(folderID)
	if err != nil {
		t.Fatal(err)
	}
	if count.Bytes != (1+2)*128<<10 {
		t.Log(count)
		t.Fatal("bad global size after drop")
	}
	if g, ok, err := db.GetGlobalFile(folderID, "test1"); err != nil || !ok {
		t.Fatal("missing global after drop")
	} else if g.Size != 1*128<<10 {
		t.Fatal("local test1 should be the global")
	}
}

func TestNeedDeleted(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		genFile("test1", 1, 0),
		genFile("test2", 2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	// A remote deleted file
	remote := []protocol.FileInfo{
		genFile("test1", 1, 101),
	}
	remote[0].SetDeleted(42)
	err = db.Update(folderID, protocol.DeviceID{42}, remote)
	if err != nil {
		t.Fatal(err)
	}

	// We need the one deleted file
	s, err := db.CountNeed(folderID, protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Bytes != 0 || s.Deleted != 1 {
		t.Log(s)
		t.Error("bad need")
	}
}

func TestDontNeedIgnored(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// A remote file
	files := []protocol.FileInfo{
		genFile("test1", 1, 103),
	}
	err = db.Update(folderID, protocol.DeviceID{42}, files)
	if err != nil {
		t.Fatal(err)
	}

	// Which we've ignored locally
	files[0].SetIgnored()
	err = db.Update(folderID, protocol.LocalDeviceID, files)
	if err != nil {
		t.Fatal(err)
	}

	// We don't need it
	s, err := db.CountNeed(folderID, protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Bytes != 0 || s.Files != 0 {
		t.Log(s)
		t.Error("bad need")
	}

	// It shouldn't show up in the need list
	names := mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0))
	if len(names) != 0 {
		t.Log(names)
		t.Error("need no files")
	}
}

func TestRemoveDontNeedLocalIgnored(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// A local ignored file
	file := genFile("test1", 1, 103)
	file.SetIgnored()
	files := []protocol.FileInfo{file}
	err = db.Update(folderID, protocol.LocalDeviceID, files)
	if err != nil {
		t.Fatal(err)
	}

	// Which the remote doesn't have (no update)

	// They don't need it
	s, err := db.CountNeed(folderID, protocol.DeviceID{42})
	if err != nil {
		t.Fatal(err)
	}
	if s.Bytes != 0 || s.Files != 0 {
		t.Log(s)
		t.Error("bad need")
	}

	// It shouldn't show up in their need list
	names := mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.DeviceID{42}, config.PullOrderAlphabetic, 0, 0))
	if len(names) != 0 {
		t.Log(names)
		t.Error("need no files")
	}
}

func TestLocalDontNeedDeletedMissing(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// A remote deleted file
	file := genFile("test1", 1, 103)
	file.SetDeleted(42)
	files := []protocol.FileInfo{file}
	err = db.Update(folderID, protocol.DeviceID{42}, files)
	if err != nil {
		t.Fatal(err)
	}

	// Which we don't have (no local update)

	// We don't need it
	s, err := db.CountNeed(folderID, protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Bytes != 0 || s.Files != 0 || s.Deleted != 0 {
		t.Log(s)
		t.Error("bad need")
	}

	// It shouldn't show up in the need list
	names := mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0))
	if len(names) != 0 {
		t.Log(names)
		t.Error("need no files")
	}
}

func TestRemoteDontNeedDeletedMissing(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// A local deleted file
	file := genFile("test1", 1, 103)
	file.SetDeleted(42)
	files := []protocol.FileInfo{file}
	err = db.Update(folderID, protocol.LocalDeviceID, files)
	if err != nil {
		t.Fatal(err)
	}

	// Which the remote doesn't have (no local update)

	// They don't need it
	s, err := db.CountNeed(folderID, protocol.DeviceID{42})
	if err != nil {
		t.Fatal(err)
	}
	if s.Bytes != 0 || s.Files != 0 || s.Deleted != 0 {
		t.Log(s)
		t.Error("bad need")
	}

	// It shouldn't show up in their need list
	names := mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.DeviceID{42}, config.PullOrderAlphabetic, 0, 0))
	if len(names) != 0 {
		t.Log(names)
		t.Error("need no files")
	}
}

func TestNeedRemoteSymlinkAndDir(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Two remote "specials", a symlink and a directory
	var v protocol.Vector
	v.Update(1)
	files := []protocol.FileInfo{
		{Name: "sym", Type: protocol.FileInfoTypeSymlink, Sequence: 100, Version: v, Blocks: genBlocks("symlink", 0, 1)},
		{Name: "dir", Type: protocol.FileInfoTypeDirectory, Sequence: 101, Version: v},
	}
	err = db.Update(folderID, protocol.DeviceID{42}, files)
	if err != nil {
		t.Fatal(err)
	}

	// We need them
	s, err := db.CountNeed(folderID, protocol.LocalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Directories != 1 || s.Symlinks != 1 {
		t.Log(s)
		t.Error("bad need")
	}

	// They should be in the need list
	names := mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0))
	if len(names) != 2 {
		t.Log(names)
		t.Error("bad need")
	}
}

func TestNeedPagination(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Several remote files
	var v protocol.Vector
	v.Update(1)
	files := []protocol.FileInfo{
		genFile("test0", 1, 100),
		genFile("test1", 1, 101),
		genFile("test2", 1, 102),
		genFile("test3", 1, 103),
		genFile("test4", 1, 104),
		genFile("test5", 1, 105),
		genFile("test6", 1, 106),
		genFile("test7", 1, 107),
		genFile("test8", 1, 108),
		genFile("test9", 1, 109),
	}
	err = db.Update(folderID, protocol.DeviceID{42}, files)
	if err != nil {
		t.Fatal(err)
	}

	// We should get the first two
	names := fiNames(mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 2, 0)))
	if !slices.Equal(names, []string{"test0", "test1"}) {
		t.Log(names)
		t.Error("bad need")
	}

	// We should get the next three
	names = fiNames(mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 3, 2)))
	if !slices.Equal(names, []string{"test2", "test3", "test4"}) {
		t.Log(names)
		t.Error("bad need")
	}

	// We should get the last five
	names = fiNames(mustCollect[protocol.FileInfo](t)(db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 5, 5)))
	if !slices.Equal(names, []string{"test5", "test6", "test7", "test8", "test9"}) {
		t.Log(names)
		t.Error("bad need")
	}
}
