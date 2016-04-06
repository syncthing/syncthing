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
	"runtime"
	"testing"

	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

func TestWriteFileNewNoSource(t *testing.T) {
	// writeFile should fail if it can't get the blocks it needs from either
	// the local or network sources.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:  "testdata",
		TempNamer: defTempNamer,
	})

	err := cs.writeFile(testFile)
	if err == nil {
		t.Error("Unexpected nil error from writeFile with no sources")
	}
}

func TestWriteFileNewErrorSource(t *testing.T) {
	// writeFile should fail if it can't get the blocks it needs from either
	// the local or network sources.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(nil),
		NetworkRequester: NewAsyncRequester(fakeRequester(nil), 4),
		TempNamer:        defTempNamer,
	})
	err := cs.writeFile(testFile)
	if err == nil {
		t.Error("Unexpected nil error from writeFile with no sources")
	}
}

func TestWriteFileNewFromLocal(t *testing.T) {
	// writeFile should succeed if all the blocks are available from the
	// local source.

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
		t.Error("Unexpected error from writeFile with local source:", err)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Error("Blocks differ after writeFile")
	}
}

func TestWriteFileNewFromNetwork(t *testing.T) {
	// writeFile should succeed if all the blocks are available from the
	// network source.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:         "testdata",
		NetworkRequester: NewAsyncRequester(fakeRequester(testBlocks[:]), 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.writeFile(testFile); err != nil {
		t.Error("Unexpected error from writeFile with local source:", err)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Error("Blocks differ after writeFile")
	}
}

func TestWriteFileNewFromMixed(t *testing.T) {
	// writeFile should succeed when all the blocks are available from a
	// combo of the local and network sources.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := verifyWrite(testFile); err != nil {
		t.Error(err)
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	// writeFile should succeed in overwriting an existing file.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test", []byte("incorrect contents"), 0777); err != nil {
		t.Fatal(err)
	}

	if err := verifyWrite(testFile); err != nil {
		t.Error(err)
	}
}

func TestWriteFileOverwriteReadOnly(t *testing.T) {
	// writeFile should succeed in overwriting an existing file that is read
	// only.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test", []byte("incorrect contents"), 0444); err != nil {
		t.Fatal(err)
	}

	if err := verifyWrite(testFile); err != nil {
		t.Error(err)
	}
}

func TestWriteFileOverwriteInReadOnlyDir(t *testing.T) {
	// writeFile should succeed in overwriting an existing file in a read
	// only directory.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/testdir", 0777); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("testdata/testdir/test", []byte("incorrect contents"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("testdata/testdir", 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/testdir", 0777)

	roFile := testFile
	roFile.Name = "testdir/test"

	if err := verifyWrite(roFile); err != nil {
		t.Error(err)
	}
}

func TestWriteFileReuseOneBlock(t *testing.T) {
	// writeFile should be able to reuse an existing temp file with one block
	// already in it.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/.syncthing.test.tmp", testBlocks[1].data, 0444); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:       "testdata",
		LocalRequester: fakeRequester(testBlocks[2:4]),
		TempNamer:      defTempNamer,
	})
	if err := cs.writeFile(testFile); err != nil {
		t.Fatal(err)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Fatal("Blocks differ after writeFile")
	}
}

func TestWriteFileReuseAll(t *testing.T) {
	// writeFile should succeed in reusing a temp file with the correct
	// contents without asking for any blocks from outside.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	fd, err := os.Create("testdata/.syncthing.test.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write(testBlocks[1].data); err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write(testBlocks[2].data); err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write(testBlocks[3].data); err != nil {
		t.Fatal(err)
	}
	if err := fd.Close(); err != nil {
		t.Fatal(err)
	}

	// The errorRequester will fail the test if writeFile tries to grab any
	// blocks from it.
	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.writeFile(testFile); err != nil {
		t.Fatal(err)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Fatal("Blocks differ after writeFile")
	}
}

func TestWriteFileReuseInReadOnlyDir(t *testing.T) {
	// writeFile should be able to reuse and overwrite a temp file with
	// incorrect data, that is not readable, in a read only directory.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/testdir", 0777); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("testdata/testdir/.syncthing.test.tmp", []byte("incorrect contents"), 0000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("testdata/testdir", 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/testdir", 0777)

	roFile := testFile
	roFile.Name = "testdir/test"

	if err := verifyWrite(roFile); err != nil {
		t.Error(err)
	}
}

func TestWriteFileMissingDirFails(t *testing.T) {
	// writeFile should not be able to write a file in a directory that
	// doesn't exist yet.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	file := testFile
	file.Name = "testdir/test"

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[1:2]),
		NetworkRequester: NewAsyncRequester(fakeRequester(testBlocks[2:]), 4),
		TempNamer:        defTempNamer,
	})

	if err := cs.writeFile(file); err == nil {
		t.Error("Unexpected nil error from writeFile")
	}
}

func TestDeleteFile(t *testing.T) {
	// deleteFile should be able to delete a file.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test", []byte("some data"), 0777); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.deleteFile(testFile); err != nil {
		t.Error(err)
	}

	if _, err := os.Lstat("testdata/test"); !os.IsNotExist(err) {
		t.Error("File still exists")
	}
}

func TestDeleteFileReadOnly(t *testing.T) {
	// deleteFile should be able to delete a file that is read only.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/testdir", 0777); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("testdata/testdir/test", []byte("some"), 0000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("testdata/testdir", 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/testdir", 0777)

	roFile := testFile
	roFile.Name = "testdir/test"

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.deleteFile(roFile); err != nil {
		t.Error(err)
	}

	if _, err := os.Lstat("testdata/testdir/test"); !os.IsNotExist(err) {
		t.Error("File still exists")
	}
}

func TestDeleteFileNoExist(t *testing.T) {
	// deleteFile should be fine with deleting a file that doesn't exist

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	file := testFile
	file.Name = "testdir/test"

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.deleteFile(file); err != nil {
		t.Error(err)
	}

	if _, err := os.Lstat("testdata/test"); !os.IsNotExist(err) {
		t.Error("File still exists")
	}
}

func TestDeleteFileInReadOnlyDir(t *testing.T) {
	// deleteFile should be able to delete a file that is in a read only
	// directory.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test", []byte("some data"), 0000); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.deleteFile(testFile); err != nil {
		t.Error(err)
	}

	if _, err := os.Lstat("testdata/test"); !os.IsNotExist(err) {
		t.Error("File still exists")
	}
}

func TestRenameFile(t *testing.T) {
	// renameFile should be able to rename a file

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := ioutil.WriteFile("testdata/test", testBlocks[0].data, 0666); err != nil {
		t.Fatal(err)
	}

	newFile := testFile
	newFile.Name = "renamed"
	newFile.Blocks = []protocol.BlockInfo{{Hash: testBlocks[0].hash}}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.renameFile(testFile, newFile); err != nil {
		t.Error(err)
	}

	if err := verifyFile("testdata", newFile); err != nil {
		t.Error(err)
	}
}

func TestRenameFileInReadOnlyDir(t *testing.T) {
	// renameFile should be able to rename a file

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/testdir", 0777); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("testdata/testdir/test", testBlocks[0].data, 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("testdata/testdir", 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/testdir", 0777)

	oldFile := testFile
	oldFile.Name = "testdir/test"

	newFile := testFile
	newFile.Name = "testdir/renamed"
	newFile.Blocks = []protocol.BlockInfo{{Hash: testBlocks[0].hash}}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   errorRequester{t},
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.renameFile(oldFile, newFile); err != nil {
		t.Error(err)
	}

	if err := verifyFile("testdata", newFile); err != nil {
		t.Error(err)
	}
}

func TestWriteSymlinkToFile(t *testing.T) {
	// writeSymlink should be able to create a symlink to an existing file

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.MkdirAll("testdata/target/of", 0777); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("testdata/target/of/symlink", []byte("a file"), 0666); err != nil {
		t.Fatal(err)
	}

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.writeSymlink(testSymlink); err != nil {
		t.Error(err)
	}

	target, targetType, err := fs.DefaultFilesystem.ReadSymlink("testdata/symlink")
	if err != nil {
		t.Error(err)
	}
	if target != "target/of/symlink" {
		t.Errorf("Incorrect target %q", target)
	}
	if targetType != fs.LinkTargetFile {
		t.Errorf("Incorrect target type %v", targetType)
	}
}

func TestFileMetadata(t *testing.T) {
	cases := []struct {
		modTime time.Time
		perm    os.FileMode
	}{
		{time.Now().Add(-25 * 365 * 24 * time.Hour).Truncate(time.Second), 0753}, // 25 years ago
		{time.Now().Add(-12 * time.Hour).Truncate(time.Second), 0512},            // 12 hours ago
		{time.Now().Add(12 * time.Hour).Truncate(time.Second), 0146},             // in 12 hours
		{time.Date(2036, 8, 24, 12, 34, 56, 0, time.Local), 0777},                // in the future, and maximally writable to make it easy for the cleanup
	}

	if runtime.GOOS == "windows" {
		// We only support 0666 and 0444 here.
		for i := range cases {
			if cases[i].perm > 0600 {
				cases[i].perm = 0666
			} else {
				cases[i].perm = 0444
			}
		}
	}

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

	for i, c := range cases {
		f := testFile
		f.Modified = c.modTime.Unix()
		f.Flags = uint32(c.perm)
		if err := cs.writeFile(f); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat("testdata/test")
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(c.modTime) {
			t.Errorf("Case %d modtimes differ: %v != %v", i, info.ModTime(), c.modTime)
		}
		if info.Mode() != c.perm {
			t.Errorf("Case %d permissions differ: 0%o != 0%o", i, info.Mode(), c.perm)
		}
	}
}

func TestWriteFileNewFromNetworkParallell(t *testing.T) {
	// This test uses the slowRequester which takes 500 ms to return a
	// response. The test file needs three blocks, but this should happen in
	// parallell so the total write time should be less than 750 ms (one
	// request roundtrip plus random overhead).

	if testing.Short() {
		t.Skip("slow test")
	}

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:         "testdata",
		NetworkRequester: NewAsyncRequester(slowRequester(testBlocks[:]), 4),
		TempNamer:        defTempNamer,
	})

	t0 := time.Now()
	if err := cs.writeFile(testFile); err != nil {
		t.Error("Unexpected error from writeFile with local source:", err)
	}
	diff := time.Since(t0)
	if diff < 500*time.Millisecond {
		// This shouldn't be possible, so there's something strange about the test.
		t.Errorf("Test finished in %v which is < 500 ms", diff)
	}
	if diff > 750*time.Millisecond {
		t.Errorf("Parallell requests should be faster than %v (> 750 ms)", diff)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Error("Blocks differ after writeFile")
	}
}

func verifyWrite(f protocol.FileInfo) error {
	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[1:2]),
		NetworkRequester: NewAsyncRequester(fakeRequester(testBlocks[2:]), 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.writeFile(f); err != nil {
		return fmt.Errorf("writeFile: %v", err)
	}

	blocks, err := scanner.HashFile(filepath.Join("testdata", f.Name), protocol.BlockSize, 0, nil)
	if err != nil {
		return fmt.Errorf("HashFile: %v", err)
	}

	if !scanner.BlocksEqual(blocks, f.Blocks) {
		return fmt.Errorf("Blocks differ after writeFile")
	}

	return nil
}
