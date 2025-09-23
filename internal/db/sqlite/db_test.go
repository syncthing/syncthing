// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"iter"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/internal/timeutil"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	folderID  = "test"
	blockSize = 128 << 10
	dirSize   = 128
)

func TestBasics(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	local := []protocol.FileInfo{
		genFile("test1", 1, 0),
		genDir("test2", 0),
		genFile("test2/a", 2, 0),
		genFile("test2/b", 3, 0),
	}
	err = sdb.Update(folderID, protocol.LocalDeviceID, local)
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	remote := []protocol.FileInfo{
		genFile("test3", 3, 101),
		genFile("test4", 4, 102),
		genFile("test1", 5, 103),
	}
	// All newer than the local ones
	for i := range remote {
		remote[i].Version = remote[i].Version.Update(42)
	}
	err = sdb.Update(folderID, protocol.DeviceID{42}, remote)
	if err != nil {
		t.Fatal(err)
	}
	const (
		localSize      = (1+2+3)*blockSize + dirSize
		remoteSize     = (3 + 4 + 5) * blockSize
		globalSize     = (2+3+3+4+5)*blockSize + dirSize
		needSizeLocal  = remoteSize
		needSizeRemote = (2+3)*blockSize + dirSize
	)

	t.Run("SchemaVersion", func(t *testing.T) {
		tx, err := sdb.sql.Beginx()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		ver, err := sdb.getAppliedSchemaVersion(tx)
		if err != nil {
			t.Fatal(err)
		}
		if ver.SchemaVersion != currentSchemaVersion {
			t.Log(ver)
			t.Error("should be version 1")
		}
		if d := time.Since(ver.AppliedTime()); d > time.Minute || d < 0 {
			t.Log(ver)
			t.Error("suspicious applied tim")
		}
	})

	t.Run("Local", func(t *testing.T) {
		t.Parallel()

		fi, ok, err := sdb.GetDeviceFile(folderID, protocol.LocalDeviceID, "test2/a") // exists
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("not found")
		}
		if fi.Name != filepath.FromSlash("test2/a") {
			t.Fatal("should have got test2/a")
		}
		if len(fi.Blocks) != 2 {
			t.Fatal("expected two blocks")
		}

		_, ok, err = sdb.GetDeviceFile(folderID, protocol.LocalDeviceID, "test3") // does not exist
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("should be not found")
		}
	})

	t.Run("Global", func(t *testing.T) {
		t.Parallel()

		fi, ok, err := sdb.GetGlobalFile(folderID, "test1")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("not found")
		}
		if fi.Size != 5*blockSize {
			t.Fatal("should be the remote file")
		}
	})

	t.Run("AllLocal", func(t *testing.T) {
		t.Parallel()

		have := mustCollect[protocol.FileInfo](t)(sdb.AllLocalFiles(folderID, protocol.LocalDeviceID))
		if len(have) != 4 {
			t.Log(have)
			t.Error("expected four files")
		}
		have = mustCollect[protocol.FileInfo](t)(sdb.AllLocalFiles(folderID, protocol.DeviceID{42}))
		if len(have) != 3 {
			t.Log(have)
			t.Error("expected three files")
		}
	})

	t.Run("AllNeededNamesLocal", func(t *testing.T) {
		t.Parallel()

		need := fiNames(mustCollect[protocol.FileInfo](t)(sdb.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0)))
		if len(need) != 3 || need[0] != "test1" {
			t.Log(need)
			t.Error("expected three files, ordered alphabetically")
		}

		need = fiNames(mustCollect[protocol.FileInfo](t)(sdb.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 1, 0)))
		if len(need) != 1 || need[0] != "test1" {
			t.Log(need)
			t.Error("expected one file, limited, ordered alphabetically")
		}
		need = fiNames(mustCollect[protocol.FileInfo](t)(sdb.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderLargestFirst, 0, 0)))
		if len(need) != 3 || need[0] != "test1" { // largest
			t.Log(need)
			t.Error("expected three files, ordered largest to smallest")
		}
		need = fiNames(mustCollect[protocol.FileInfo](t)(sdb.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderSmallestFirst, 0, 0)))
		if len(need) != 3 || need[0] != "test3" { // smallest
			t.Log(need)
			t.Error("expected three files, ordered smallest to largest")
		}

		need = fiNames(mustCollect[protocol.FileInfo](t)(sdb.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderNewestFirst, 0, 0)))
		if len(need) != 3 || need[0] != "test1" { // newest
			t.Log(need)
			t.Error("expected three files, ordered newest to oldest")
		}
		need = fiNames(mustCollect[protocol.FileInfo](t)(sdb.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderOldestFirst, 0, 0)))
		if len(need) != 3 || need[0] != "test3" { // oldest
			t.Log(need)
			t.Error("expected three files, ordered oldest to newest")
		}
	})

	t.Run("LocalSize", func(t *testing.T) {
		t.Parallel()

		// Local device

		c, err := sdb.CountLocal(folderID, protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if c.Files != 3 {
			t.Log(c)
			t.Error("one file expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != localSize {
			t.Log(c)
			t.Error("size unexpected")
		}

		// Other device

		c, err = sdb.CountLocal(folderID, protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if c.Files != 3 {
			t.Log(c)
			t.Error("three files expected")
		}
		if c.Directories != 0 {
			t.Log(c)
			t.Error("no directories expected")
		}
		if c.Bytes != remoteSize {
			t.Log(c)
			t.Error("size unexpected")
		}
	})

	t.Run("GlobalSize", func(t *testing.T) {
		t.Parallel()

		c, err := sdb.CountGlobal(folderID)
		if err != nil {
			t.Fatal(err)
		}
		if c.Files != 5 {
			t.Log(c)
			t.Error("five files expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != int64(globalSize) {
			t.Log(c)
			t.Error("size unexpected")
		}
	})

	t.Run("NeedSizeLocal", func(t *testing.T) {
		t.Parallel()

		c, err := sdb.CountNeed(folderID, protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if c.Files != 3 {
			t.Log(c)
			t.Error("three files expected")
		}
		if c.Directories != 0 {
			t.Log(c)
			t.Error("no directories expected")
		}
		if c.Bytes != needSizeLocal {
			t.Log(c)
			t.Error("size unexpected")
		}
	})

	t.Run("NeedSizeRemote", func(t *testing.T) {
		t.Parallel()

		c, err := sdb.CountNeed(folderID, protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if c.Files != 2 {
			t.Log(c)
			t.Error("two files expected")
		}
		if c.Directories != 1 {
			t.Log(c)
			t.Error("one directory expected")
		}
		if c.Bytes != needSizeRemote {
			t.Log(c)
			t.Error("size unexpected")
		}
	})

	t.Run("Folders", func(t *testing.T) {
		t.Parallel()

		folders, err := sdb.ListFolders()
		if err != nil {
			t.Fatal(err)
		}
		if len(folders) != 1 || folders[0] != folderID {
			t.Log(folders)
			t.Error("expected one folder")
		}
	})

	t.Run("DevicesForFolder", func(t *testing.T) {
		t.Parallel()

		devs, err := sdb.ListDevicesForFolder("test")
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

		iid, err := sdb.GetIndexID(folderID, protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if iid == 0 {
			t.Log(iid)
			t.Fatal("expected index ID")
		}

		if seq, err := sdb.GetDeviceSequence(folderID, protocol.LocalDeviceID); err != nil {
			t.Fatal(err)
		} else if seq != 4 {
			t.Log(seq)
			t.Error("expected local sequence to match number of files inserted")
		}

		if seq, err := sdb.GetDeviceSequence(folderID, protocol.DeviceID{42}); err != nil {
			t.Fatal(err)
		} else if seq != 103 {
			t.Log(seq)
			t.Error("expected remote sequence to match highest sent")
		}

		// Non-existent should be zero and no error
		if seq, err := sdb.GetDeviceSequence("trolol", protocol.LocalDeviceID); err != nil {
			t.Fatal(err)
		} else if seq != 0 {
			t.Log(seq)
			t.Error("expected zero sequence")
		}
		if seq, err := sdb.GetDeviceSequence("trolol", protocol.DeviceID{42}); err != nil {
			t.Fatal(err)
		} else if seq != 0 {
			t.Log(seq)
			t.Error("expected zero sequence")
		}
		if seq, err := sdb.GetDeviceSequence(folderID, protocol.DeviceID{99}); err != nil {
			t.Fatal(err)
		} else if seq != 0 {
			t.Log(seq)
			t.Error("expected zero sequence")
		}
	})

	t.Run("AllGlobalPrefix", func(t *testing.T) {
		t.Parallel()

		vals := mustCollect[db.FileMetadata](t)(sdb.AllGlobalFilesPrefix(folderID, "test2"))

		// Vals should be test2, test2/a, test2/b
		if len(vals) != 3 {
			t.Log(vals)
			t.Error("expected three items")
		} else if vals[0].Name != "test2" {
			t.Error(vals)
		}

		// Empty prefix should be all the files
		vals = mustCollect[db.FileMetadata](t)(sdb.AllGlobalFilesPrefix(folderID, ""))
		if len(vals) != 6 {
			t.Log(vals)
			t.Error("expected six items")
		}
	})

	t.Run("AllLocalPrefix", func(t *testing.T) {
		t.Parallel()

		vals := mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, "test2"))

		// Vals should be test2, test2/a, test2/b
		if len(vals) != 3 {
			t.Log(vals)
			t.Error("expected three items")
		} else if vals[0].Name != "test2" {
			t.Error(vals)
		}

		// Empty prefix should be all the files
		vals = mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, ""))

		if len(vals) != 4 {
			t.Log(vals)
			t.Error("expected four items")
		}
	})

	t.Run("AllLocalSequenced", func(t *testing.T) {
		t.Parallel()

		vals := mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesBySequence(folderID, protocol.LocalDeviceID, 3, 0))

		// Vals should be test2/a, test2/b
		if len(vals) != 2 {
			t.Log(vals)
			t.Error("expected three items")
		} else if vals[0].Name != filepath.FromSlash("test2/a") || vals[0].Sequence != 3 {
			t.Error(vals)
		}
	})
}

func TestPrefixGlobbing(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	local := []protocol.FileInfo{
		genFile("test1", 1, 0),
		genDir("test2", 0),
		genFile("test2/a", 2, 0),
		genDir("test2/b", 0),
		genFile("test2/b/c", 3, 0),
	}
	err = sdb.Update(folderID, protocol.LocalDeviceID, local)
	if err != nil {
		t.Fatal(err)
	}

	vals := mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, "test2"))

	// Vals should be test2, test2/a, test2/b, test2/b/c
	if len(vals) != 4 {
		t.Log(vals)
		t.Error("expected four items")
	} else if vals[0].Name != "test2" || vals[3].Name != filepath.FromSlash("test2/b/c") {
		t.Error(vals)
	}

	// Empty prefix should be all the files
	vals = mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, ""))

	if len(vals) != 5 {
		t.Log(vals)
		t.Error("expected five items")
	}

	// Same as partial prefix
	vals = mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, "tes"))

	if len(vals) != 5 {
		t.Log(vals)
		t.Error("expected five items")
	}

	// Prefix should be case sensitive, so no match here
	vals = mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, "tEsT2"))

	if len(vals) != 0 {
		t.Log(vals)
		t.Error("expected no items")
	}

	// Subdir should match
	vals = mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, "test2/b"))

	if len(vals) != 2 {
		t.Log(vals)
		t.Error("expected two items")
	}
}

func TestPrefixGlobbingStar(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files
	local := []protocol.FileInfo{
		genFile("test1a", 1, 0),
		genFile("test*a", 2, 0),
		genFile("test2a", 3, 0),
	}
	err = sdb.Update(folderID, protocol.LocalDeviceID, local)
	if err != nil {
		t.Fatal(err)
	}

	vals := mustCollect[protocol.FileInfo](t)(sdb.AllLocalFilesWithPrefix(folderID, protocol.LocalDeviceID, "test*a"))

	// Vals should be test*a
	if len(vals) != 1 {
		t.Log(vals)
		t.Error("expected one item")
	} else if vals[0].Name != "test*a" {
		t.Error(vals)
	}
}

func TestAvailability(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const folderID = "test"

	// Some local files
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		genFile("test1", 1, 0),
		genFile("test2", 2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Some remote files
	err = db.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{
		genFile("test2", 2, 1),
		genFile("test3", 3, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Further remote files
	err = db.Update(folderID, protocol.DeviceID{45}, []protocol.FileInfo{
		genFile("test3", 3, 1),
		genFile("test4", 4, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	a, err := db.GetGlobalAvailability(folderID, "test1")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 0 {
		t.Log(a)
		t.Error("expected no availability (only local)")
	}

	a, err = db.GetGlobalAvailability(folderID, "test2")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0] != (protocol.DeviceID{42}) {
		t.Log(a)
		t.Error("expected one availability (only 42)")
	}

	a, err = db.GetGlobalAvailability(folderID, "test3")
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
	db, err := Open(t.TempDir())
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
	err = db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{
		genFile("test1", 1, 0),
		genFile("test2", 2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop test1
	if err := db.DropFilesNamed(folderID, protocol.LocalDeviceID, []string{"test1"}); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.GetDeviceFile(folderID, protocol.LocalDeviceID, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c, err := db.CountLocal(folderID, protocol.LocalDeviceID); err != nil {
		t.Fatal(err)
	} else if c.Files != 1 {
		t.Log(c)
		t.Error("expected count to be one")
	}
	if _, ok, err := db.GetDeviceFile(folderID, protocol.LocalDeviceID, "test2"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
}

func TestDropFolder(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files

	// Folder A
	err = db.Update("a", protocol.LocalDeviceID, []protocol.FileInfo{
		genFile("test1", 1, 0),
		genFile("test2", 2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Folder B
	err = db.Update("b", protocol.LocalDeviceID, []protocol.FileInfo{
		genFile("test1", 1, 0),
		genFile("test2", 2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop A
	if err := db.DropFolder("a"); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.GetDeviceFile("a", protocol.LocalDeviceID, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c, err := db.CountLocal("a", protocol.LocalDeviceID); err != nil {
		t.Fatal(err)
	} else if c.Files != 0 {
		t.Log(c)
		t.Error("expected count to be zero")
	}

	if _, ok, err := db.GetDeviceFile("b", protocol.LocalDeviceID, "test1"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
	if c, err := db.CountLocal("b", protocol.LocalDeviceID); err != nil {
		t.Fatal(err)
	} else if c.Files != 2 {
		t.Log(c)
		t.Error("expected count to be two")
	}
}

func TestDropDevice(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files

	// Device 1
	err = db.Update("a", protocol.DeviceID{1}, []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Device 2
	err = db.Update("a", protocol.DeviceID{2}, []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop 1
	if err := db.DropDevice(protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.GetDeviceFile("a", protocol.DeviceID{1}, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c, err := db.CountLocal("a", protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	} else if c.Files != 0 {
		t.Log(c)
		t.Error("expected count to be zero")
	}
	if _, ok, err := db.GetDeviceFile("a", protocol.DeviceID{2}, "test1"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
	if c, err := db.CountLocal("a", protocol.DeviceID{2}); err != nil {
		t.Fatal(err)
	} else if c.Files != 2 {
		t.Log(c)
		t.Error("expected count to be two")
	}

	// Drop something that doesn't exist
	if err := db.DropDevice(protocol.DeviceID{99}); err != nil {
		t.Fatal(err)
	}
}

func TestDropAllFiles(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Some local files

	// Device 1 folder A
	err = db.Update("a", protocol.DeviceID{1}, []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Device 1 folder B
	err = db.Update("b", protocol.DeviceID{1}, []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drop folder A
	if err := db.DropAllFiles("a", protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	}

	// Check
	if _, ok, err := db.GetDeviceFile("a", protocol.DeviceID{1}, "test1"); err != nil || ok {
		t.Log(err, ok)
		t.Error("expected to not exist")
	}
	if c, err := db.CountLocal("a", protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	} else if c.Files != 0 {
		t.Log(c)
		t.Error("expected count to be zero")
	}
	if _, ok, err := db.GetDeviceFile("b", protocol.DeviceID{1}, "test1"); err != nil || !ok {
		t.Log(err, ok)
		t.Error("expected to exist")
	}
	if c, err := db.CountLocal("b", protocol.DeviceID{1}); err != nil {
		t.Fatal(err)
	} else if c.Files != 2 {
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

	files := []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
		genFile("test3", 3, 3),
		genFile("test4", 4, 4),
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
	files := []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
		genFile("test3", 3, 3),
		genFile("test4", 4, 4),
	}

	// Insert the files for a remote device
	if err := db.Update(folderID, protocol.DeviceID{42}, files); err != nil {
		t.Fatal()
	}

	// Iterate over handled files and insert them for the local device.
	// This is similar to a pattern we have in other places and should
	// work.
	handled := 0
	it, errFn := db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0)
	for glob := range it {
		glob.Version = glob.Version.Update(1)
		if err := db.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{glob}); err != nil {
			t.Fatal(err)
		}
		handled++
	}
	if err := errFn(); err != nil {
		t.Fatal(err)
	}

	if handled != len(files) {
		t.Log(handled)
		t.Error("should have handled all the files")
	}
}

func TestAllForBlocksHash(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// test1 is unique, while test2 and test3 have the same blocks and hence
	// the same blocks hash

	files := []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
		genFile("test3", 3, 3),
	}
	files[2].Blocks = files[1].Blocks

	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Check test1

	test1, ok, err := sdb.GetDeviceFile(folderID, protocol.LocalDeviceID, "test1")
	if err != nil || !ok {
		t.Fatal("expected to exist")
	}

	vals := mustCollect[db.FileMetadata](t)(sdb.AllLocalFilesWithBlocksHash(folderID, test1.BlocksHash))
	if len(vals) != 1 {
		t.Log(vals)
		t.Fatal("expected one file to match")
	}

	// Check test2 which also matches test3

	test2, ok, err := sdb.GetDeviceFile(folderID, protocol.LocalDeviceID, "test2")
	if err != nil || !ok {
		t.Fatal("expected to exist")
	}

	vals = mustCollect[db.FileMetadata](t)(sdb.AllLocalFilesWithBlocksHash(folderID, test2.BlocksHash))
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

func TestBlocklistGarbageCollection(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})
	svc := sdb.Service(time.Hour).(*Service)

	// Add three files

	files := []protocol.FileInfo{
		genFile("test1", 1, 1),
		genFile("test2", 2, 2),
		genFile("test3", 3, 3),
	}

	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// There should exist three blockslists and six blocks

	fdb, err := sdb.getFolderDB(folderID, false)
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := fdb.sql.Get(&count, `SELECT count(*) FROM blocklists`); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Log(count)
		t.Fatal("expected 3 blocklists")
	}
	if err := fdb.sql.Get(&count, `SELECT count(*) FROM blocks`); err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Log(count)
		t.Fatal("expected 6 blocks")
	}

	// Mark test3 as deleted, it's blocks and blocklist are now eligible for collection
	files = files[2:]
	files[0].SetDeleted(42)
	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Run garbage collection
	if err := svc.periodic(context.Background()); err != nil {
		t.Fatal(err)
	}

	// There should exist two blockslists and four blocks

	if err := fdb.sql.Get(&count, `SELECT count(*) FROM blocklists`); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Log(count)
		t.Error("expected 2 blocklists")
	}
	if err := fdb.sql.Get(&count, `SELECT count(*) FROM blocks`); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Log(count)
		t.Error("expected 3 blocks")
	}
}

func TestInsertLargeFile(t *testing.T) {
	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Add a large file (many blocks)

	files := []protocol.FileInfo{genFile("test1", 16000, 1)}
	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Verify all the blocks are here

	for i, block := range files[0].Blocks {
		bs, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, block.Hash))
		if err != nil {
			t.Fatal(err)
		}
		if len(bs) == 0 {
			t.Error("missing blocks for", i)
		}
	}
}

func TestErrorWrap(t *testing.T) {
	if wrap(nil, "foo") != nil {
		t.Fatal("nil should wrap to nil")
	}

	fooErr := errors.New("foo")
	if err := wrap(fooErr); err.Error() != "testerrorwrap: foo" {
		t.Fatalf("%q", err)
	}

	if err := wrap(fooErr, "bar", "baz"); err.Error() != "testerrorwrap (bar, baz): foo" {
		t.Fatalf("%q", err)
	}
}

func TestStrangeDeletedGlobalBug(t *testing.T) {
	// This exercises an edge case with serialisation and ordering of
	// version vectors. It does not need to make sense, it just needs to
	// pass.

	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// One remote device announces the original version of the file

	file := genFile("test", 1, 1)
	file.Version = protocol.Vector{Counters: []protocol.Counter{{ID: 35494436325452, Value: 1742900373}}}
	t.Log("orig", file.Version)
	sdb.Update(folderID, protocol.DeviceID{42}, []protocol.FileInfo{file})

	// Another one announces a newer one that is deleted

	del := file
	del.SetDeleted(43)
	del.Version = protocol.Vector{Counters: []protocol.Counter{{ID: 55445057455644, Value: 1742918457}, {ID: 35494436325452, Value: 1742900373}}}
	t.Log("del", del.Version)
	sdb.Update(folderID, protocol.DeviceID{43}, []protocol.FileInfo{del})

	// We have an instance of the original file

	sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file})

	// Which one is the global? It should be the deleted one, clearly.

	g, _, err := sdb.GetGlobalFile(folderID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Deleted {
		t.Log(g)
		t.Fatal("should be deleted")
	}
}

func TestOpenSpecialName(t *testing.T) {
	dir := t.TempDir()

	// Create a "base" dir that is in the way if the path becomes
	// incorrectly truncated in the next steps.
	base := path.Join(dir, "test")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}

	// Should be able to open a path with a hash sign in it.
	p1 := base + "#foo"
	db, err := Open(p1)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(db.path)
	db.Close()

	if !build.IsWindows {
		// Should be able to open a path with something that looks like
		// query params.
		p2 := base + "?foo=bar"
		db, err = Open(p2)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(db.path)
		db.Close()
	}

	// Better not a have problem with a single ampersand either.
	p2 := base + "&foo"
	db, err = Open(p2)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(db.path)
	db.Close()
}

func TestBlocksInserted(t *testing.T) {
	// Verifies that blocks are inserted after syncing a file

	t.Parallel()

	sdb, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sdb.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// Add remote file

	files := []protocol.FileInfo{genFile("test1", 100, 1)}
	if err := sdb.Update(folderID, protocol.DeviceID{42}, files); err != nil {
		t.Fatal(err)
	}

	// Add the same file locally

	if err := sdb.Update(folderID, protocol.LocalDeviceID, files); err != nil {
		t.Fatal(err)
	}

	// Verify all the blocks are here

	for i, block := range files[0].Blocks {
		bs, err := itererr.Collect(sdb.AllLocalBlocksWithHash(folderID, block.Hash))
		if err != nil {
			t.Fatal(err)
		}
		if len(bs) == 0 {
			t.Error("missing blocks for", i)
		}
	}
}

func mustCollect[T any](t *testing.T) func(it iter.Seq[T], errFn func() error) []T {
	t.Helper()
	return func(it iter.Seq[T], errFn func() error) []T {
		t.Helper()
		vals, err := itererr.Collect(it, errFn)
		if err != nil {
			t.Fatal(err)
		}
		return vals
	}
}

func fiNames(fs []protocol.FileInfo) []string {
	names := make([]string, len(fs))
	for i, fi := range fs {
		names[i] = fi.Name
	}
	return names
}

func genDir(name string, seq int) protocol.FileInfo {
	return protocol.FileInfo{
		Name:        name,
		Type:        protocol.FileInfoTypeDirectory,
		ModifiedS:   time.Now().Unix(),
		ModifiedBy:  1,
		Sequence:    int64(seq),
		Version:     protocol.Vector{}.Update(1),
		Permissions: 0o755,
		ModifiedNs:  12345678,
	}
}

func genFile(name string, numBlocks int, seq int) protocol.FileInfo {
	ts := timeutil.StrictlyMonotonicNanos()
	s := ts / 1e9
	ns := int32(ts % 1e9)
	return protocol.FileInfo{
		Name:         name,
		Size:         int64(numBlocks) * blockSize,
		ModifiedS:    s,
		ModifiedBy:   1,
		Version:      protocol.Vector{}.Update(1),
		Sequence:     int64(seq),
		Blocks:       genBlocks(name, 0, numBlocks),
		Permissions:  0o644,
		ModifiedNs:   ns,
		RawBlockSize: blockSize,
	}
}

func genBlocks(name string, seed, count int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, count)
	for i := range b {
		b[i].Hash = genBlockHash(name, seed, i)
		b[i].Size = blockSize
		b[i].Offset = (blockSize) * int64(i)
	}
	return b
}

func genBlockHash(name string, seed, index int) []byte {
	bs := sha256.Sum256([]byte(name))
	ebs := binary.LittleEndian.AppendUint64(nil, uint64(seed))
	for i := range ebs {
		bs[i] ^= ebs[i]
	}
	ebs = binary.LittleEndian.AppendUint64(nil, uint64(index))
	for i := range ebs {
		bs[i] ^= ebs[i]
	}
	return bs[:]
}
