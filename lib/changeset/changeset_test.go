// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

func TestChangeSetCreateDeleteFiles(t *testing.T) {
	// Apply should create two files, delete a third

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test3", []byte("some data"), 0666); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})

	cs.Queue(testFile)
	cs.Queue(testFile2)
	cs.Queue(protocol.FileInfo{
		Name:  "test3",
		Flags: protocol.FlagDeleted,
	})

	if cs.Size() != 3 {
		t.Errorf("Incorrect size %d != 3", cs.Size())
	}

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	if err := verifyFile("testdata", testFile); err != nil {
		t.Error(err)
	}
	if err := verifyFile("testdata", testFile2); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat("testdata/test3"); !os.IsNotExist(err) {
		t.Error("test3 was not deleted")
	}
}

func TestChangeSetCreateDeleteDirs(t *testing.T) {
	// Apply should create a directory, delete another

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/foo", 0777); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:  "testdata",
		TempNamer: defTempNamer,
	})

	delDir := protocol.FileInfo{
		Name:  "foo",
		Flags: protocol.FlagDirectory | protocol.FlagDeleted,
	}

	cs.Queue(testDir)
	cs.Queue(delDir)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}

	if _, err := os.Lstat("testdata/foo"); !os.IsNotExist(err) {
		t.Error("foo was not deleted")
	}
	if _, err := os.Lstat("testdata/dir"); err != nil {
		t.Error("dir does not exist?", err)
	}
}

func TestChangeSetRenameFiles(t *testing.T) {
	// Apply should rename a file without requesting any blocks, and with correct progress accounting

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:       "testdata",
		LocalRequester: fakeRequester(testBlocks[:]),
		TempNamer:      defTempNamer,
	})

	// Make sure the source file exists
	if err := cs.writeFile(testFile); err != nil {
		t.Fatal(err)
	}

	// Create a new change set and queue the rename

	cp := new(countingProgresser)
	cs = New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
		Progresser:       cp,
	})

	delFile := testFile
	delFile.Flags |= protocol.FlagDeleted

	newFile := testFile
	newFile.Name = "test2"

	cs.Queue(delFile)
	cs.Queue(newFile)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	if _, err := os.Lstat("testdata/test"); !os.IsNotExist(err) {
		t.Error("test was not deleted")
	}
	if err := verifyFile("testdata", newFile); err != nil {
		t.Error(err)
	}

	if cp.started != 2 {
		t.Errorf("Incorrect started count, %d != 2", cp.started)
	}
	if cp.completed != 2 {
		t.Errorf("Incorrect completed count, %d != 2", cp.completed)
	}
	if cp.copied != 0 {
		t.Errorf("Incorrect copied count, %d != 0", cp.copied)
	}
	if cp.downloaded != 0 {
		t.Errorf("Incorrect downloaded count, %d != 0", cp.downloaded)
	}
}

func TestChangeSetCreateDeleteSymlinks(t *testing.T) {
	// Apply should create one symlink, delete another

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.MkdirAll("testdata/target/of/symlink", 0777); err != nil {
		t.Fatal(err)
	}
	if err := fs.DefaultFilesystem.CreateSymlink("testdata/symToDelete", "target/of/symlink", fs.LinkTargetDirectory); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})

	testNewSym := testSymlink
	testNewSym.Name = "newSymlink"
	testNewSym.Flags |= protocol.FlagDirectory

	testDelSym := testSymlink
	testDelSym.Name = "symToDelete"
	testDelSym.Flags |= protocol.FlagDirectory | protocol.FlagDeleted

	cs.Queue(testNewSym)
	cs.Queue(testDelSym)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	target, targetType, err := fs.DefaultFilesystem.ReadSymlink("testdata/newSymlink")
	if err != nil {
		t.Error(err)
	}
	if target != "target/of/symlink" {
		t.Errorf("Incorrect target %q", target)
	}
	if targetType != fs.LinkTargetDirectory {
		t.Errorf("Incorrect target type %v", targetType)
	}
	if _, err := os.Lstat("testdata/symToDelete"); !os.IsNotExist(err) {
		t.Error("symToDelete was not deleted")
	}
}

type fakeCurrentFiler protocol.FileInfo

func (f fakeCurrentFiler) CurrentFile(name string) (protocol.FileInfo, bool) {
	if name == f.Name {
		return protocol.FileInfo(f), true
	}
	return protocol.FileInfo{}, false
}

func TestChangeSetMustRescan(t *testing.T) {
	// If the CurrentFiler returns a file that is not the one we are about to
	// replace, the file should not be replaced and we should return an error
	// indicating that the folder must be rescanned.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	// Some stuff in the test file
	if err := ioutil.WriteFile("testdata/test", testBlocks[0].data, 0666); err != nil {
		t.Fatal(err)
	}

	// The old file with a modtime not matching the one from above, but the
	// hash(data) does actually match (we should not look at it...)
	oldFile := testFile
	oldFile.Modified = time.Now().Add(-60 * time.Second).Unix()
	oldFile.Blocks = []protocol.BlockInfo{protocol.BlockInfo{Size: protocol.BlockSize, Hash: testBlocks[0].hash}}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
		CurrentFiler:     fakeCurrentFiler(oldFile),
	})

	cs.Queue(testFile)

	err := cs.Apply()
	if err == nil {
		t.Error("Unexpected nil error from Apply")
	} else {
		err := err.(ApplyError)
		if !err.MustRescan() {
			t.Error("Should have been told to rescan")
		}
	}

	if blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil); err != nil {
		t.Error("Surprising error from scanner:", err)
	} else if !scanner.BlocksEqual(blocks, oldFile.Blocks) {
		t.Error("File was changed when it shouldn't have been")
	}
}

type fakeArchiver struct {
	requests []string
}

func (f *fakeArchiver) Archive(file string) error {
	f.requests = append(f.requests, file)
	return nil
}

func TestChangeSetVersionFile(t *testing.T) {
	// If the archiver is set, it should be called to replace files.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	// Some stuff in the test files
	if err := ioutil.WriteFile("testdata/test", testBlocks[0].data, 0666); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("testdata/test2", testBlocks[0].data, 0666); err != nil {
		t.Fatal(err)
	}

	fv := new(fakeArchiver)
	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
		Archiver:         fv,
	})

	cs.Queue(testFile)

	delFile := testFile
	delFile.Name = "test2"
	delFile.Flags |= protocol.FlagDeleted
	delFile.Blocks = nil
	cs.Queue(delFile)

	err := cs.Apply()
	if err != nil {
		t.Error(err.(ApplyError).Errors())
	}

	if len(fv.requests) != 2 {
		t.Errorf("Incorrect number of Archive calls, %d != 2", len(fv.requests))
	}
}

func verifyFile(rootPath string, f protocol.FileInfo) error {
	blocks, err := scanner.HashFile(filepath.Join(rootPath, f.Name), protocol.BlockSize, 0, nil)
	if err != nil {
		return fmt.Errorf("HashFile: %s: %v", f.Name, err)
	}

	if !scanner.BlocksEqual(blocks, f.Blocks) {
		return fmt.Errorf("Blocks differ: %s", f.Name)
	}

	return nil
}

func TestChangeSetProgresser(t *testing.T) {
	// The progresser should be updated with what's going on

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test3", []byte("some data"), 0666); err != nil {
		t.Fatal(err)
	}

	cp := new(countingProgresser)
	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:4]),
		NetworkRequester: NewAsyncRequester(fakeRequester(testBlocks[4:]), 4),
		TempNamer:        defTempNamer,
		Progresser:       cp,
	})

	cs.Queue(testFile)    // A created file
	cs.Queue(testFile2)   // Another one
	cs.Queue(testDir)     // A created dir
	cs.Queue(testSymlink) // A created symlink
	// A deleted file
	cs.Queue(protocol.FileInfo{
		Name:  "test3",
		Flags: protocol.FlagDeleted,
	})
	// A deleted dir
	cs.Queue(protocol.FileInfo{
		Name:  "test4",
		Flags: protocol.FlagDeleted | protocol.FlagDirectory,
	})
	// A deleted symlink
	cs.Queue(protocol.FileInfo{
		Name:  "test5",
		Flags: protocol.FlagDeleted | protocol.FlagSymlink,
	})

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}

	if cp.started != 7 {
		t.Errorf("Incorrect started count %d != 7", cp.started)
	}
	if cp.completed != 7 {
		t.Errorf("Incorrect completed count %d != 7", cp.completed)
	}
	if cp.copied != 4*protocol.BlockSize {
		t.Errorf("Incorrect copied amount %d != 4*BlockSize", cp.copied)
	}
	if cp.downloaded != 2*protocol.BlockSize {
		t.Errorf("Incorrect downloaded amount %d != 2*BlockSize", cp.downloaded)
	}
}

func TestChangeSetReplaceFileWithDir(t *testing.T) {
	// Remove a file and create a directory in it's place. This is complicated
	// by the fact that we only get the "new" state. The old state can be
	// queried from the CurrentFiler though.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:       "testdata",
		LocalRequester: fakeRequester(testBlocks[:]),
		TempNamer:      defTempNamer,
	})
	if err := cs.writeFile(testFile); err != nil {
		t.Fatal(err)
	}

	cs = New(Options{
		RootPath:       "testdata",
		LocalRequester: fakeRequester(testBlocks[:]),
		TempNamer:      defTempNamer,
		CurrentFiler:   fakeCurrentFiler(testFile),
	})

	td := testDir
	td.Name = "test" // The name of the file we created above
	cs.Queue(td)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	if info, err := os.Lstat("testdata/test"); err != nil || !info.IsDir() {
		t.Error("test is not a dir")
	}
}

func TestChangeSetReplaceDirWithFile(t *testing.T) {
	// Remove a directory and create a file in it's place

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/test", 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("testdata/test/lower1", 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("testdata/test/lower2", 0777); err != nil {
		t.Fatal(err)
	}
	td := testDir
	td.Name = "test"

	cp := new(countingProgresser)
	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
		CurrentFiler:     fakeCurrentFiler(td),
		Progresser:       cp,
	})

	cs.Queue(testFile)

	td.Flags |= protocol.FlagDeleted
	td.Name = "test/lower1"
	cs.Queue(td)
	td.Name = "test/lower2"
	cs.Queue(td)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	if err := verifyFile("testdata", testFile); err != nil {
		t.Error(err)
	}
	if cp.completed != 3 {
		t.Errorf("Unexpected completed %d != queued 3", cp.completed)
	}
}

func TestChangeSetFileConflict(t *testing.T) {
	// Update a file in conflict and verify that it creates a conflict copy

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})

	// Make sure the source file exists
	if err := cs.writeFile(testFile); err != nil {
		t.Fatal(err)
	}

	oldFile := testFile
	oldFile.Version = protocol.Vector{{1, 2}, {3, 4}}

	newFile := testFile
	newFile.Version = protocol.Vector{{1, 3}, {3, 3}} // in conflict with oldFile
	newFile.Blocks = []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}}

	cs = New(Options{
		RootPath:         "testdata",
		MaxConflicts:     -1,
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
		CurrentFiler:     fakeCurrentFiler(oldFile),
	})

	cs.Queue(newFile)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	if err := verifyFile("testdata", newFile); err != nil {
		t.Error(err)
	}
	if files, err := filepath.Glob("testdata/test.sync-conflict-*"); err != nil {
		t.Error(err)
	} else if len(files) != 1 {
		t.Errorf("Incorrect number of conflict files %d != 1", len(files))
	}
}

func TestChangeSetFileConflictOnConflict(t *testing.T) {
	// Conflicts on sync conflicts should be ignored

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})

	conflict := testFile
	conflict.Name = "test.sync-conflict-20160101-000000"

	// Make sure the source file exists
	if err := cs.writeFile(conflict); err != nil {
		t.Fatal(err)
	}

	oldFile := conflict
	oldFile.Version = protocol.Vector{{1, 2}, {3, 4}}

	newFile := conflict
	newFile.Version = protocol.Vector{{1, 3}, {3, 3}} // in conflict with oldFile
	newFile.Blocks = []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}}

	cs = New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
		CurrentFiler:     fakeCurrentFiler(oldFile),
	})

	cs.Queue(newFile)

	if err := cs.Apply(); err != nil {
		t.Error(err.(ApplyError).Errors())
	}
	if err := verifyFile("testdata", newFile); err != nil {
		t.Error(err)
	}
	if files, err := filepath.Glob("testdata/test.sync-conflict-*"); err != nil {
		t.Error(err)
	} else if len(files) != 1 {
		t.Errorf("Incorrect number of conflict files %d != 1", len(files))
	}
	if files, err := filepath.Glob("testdata/test.sync-conflict-*.sync-conflict-*"); err != nil {
		t.Error(err)
	} else if len(files) != 0 {
		t.Errorf("Incorrect number ofdouble conflict files %d != 0", len(files))
	}
}
