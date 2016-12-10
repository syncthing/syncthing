// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestRequestSimple(t *testing.T) {
	// Verify that the model performs a request and creates a file based on
	// an incoming index update.

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnection()
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if f.Name == "testfile" {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	contents := []byte("test file contents\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, contents)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err := ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not sync correctly:", err)
		return
	}
	if !bytes.Equal(bs, contents) {
		t.Error("File did not sync correctly: incorrect data")
	}
}

func TestSymlinkTraversalRead(t *testing.T) {
	// Verify that a symlink can not be traversed for reading.

	if runtime.GOOS == "windows" {
		t.Skip("no symlink support on CI")
		return
	}

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnection()
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if f.Name == "symlink" {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the symlink, wait for it to sync and be reported back.
	contents := []byte("..")
	fc.addFile("symlink", 0644, protocol.FileInfoTypeSymlinkDirectory, contents)
	fc.sendIndexUpdate()
	<-done

	// Request a file by traversing the symlink
	buf := make([]byte, 10)
	err := m.Request(device1, "default", "symlink/requests_test.go", 0, nil, false, buf)
	if err == nil || !bytes.Equal(buf, make([]byte, 10)) {
		t.Error("Managed to traverse symlink")
	}
}

func TestSymlinkTraversalWrite(t *testing.T) {
	// Verify that a symlink can not be traversed for writing.

	if runtime.GOOS == "windows" {
		t.Skip("no symlink support on CI")
		return
	}

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnection()
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected names.
	done := make(chan struct{})
	badReq := make(chan string)
	badIdx := make(chan string)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if f.Name == "symlink" {
				close(done)
				return
			}
			if strings.HasPrefix(f.Name, "symlink") {
				badIdx <- f.Name
				return
			}
		}
	}
	fc.requestFn = func(folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error) {
		if name != "symlink" && strings.HasPrefix(name, "symlink") {
			badReq <- name
		}
		return fc.fileData[name], nil
	}
	fc.mut.Unlock()

	// Send an update for the symlink, wait for it to sync and be reported back.
	contents := []byte("..")
	fc.addFile("symlink", 0644, protocol.FileInfoTypeSymlinkDirectory, contents)
	fc.sendIndexUpdate()
	<-done

	// Send an update for things behind the symlink, wait for requests for
	// blocks for any of them to come back, or index entries. Hopefully none
	// of that should happen.
	contents = []byte("testdata testdata\n")
	fc.addFile("symlink/testfile", 0644, protocol.FileInfoTypeFile, contents)
	fc.addFile("symlink/testdir", 0644, protocol.FileInfoTypeDirectory, contents)
	fc.addFile("symlink/testsyml", 0644, protocol.FileInfoTypeSymlinkFile, contents)
	fc.sendIndexUpdate()

	select {
	case name := <-badReq:
		t.Fatal("Should not have requested the data for", name)
	case name := <-badIdx:
		t.Fatal("Should not have sent the index entry for", name)
	case <-time.After(3 * time.Second):
		// Unfortunately not much else to trigger on here. The puller sleep
		// interval is 1s so if we didn't get any requests within two
		// iterations we should be fine.
	}
}

func TestSymlinkReplaceTraversalWrite(t *testing.T) {
	// Verify that a symlink can not be traversed for writing, when
	// replacing a directory. This triggers a race condition where we send
	// both a replacement dir->symlink and then a file behind the symlink
	// (previously dir). Symlinks are processed as files, which means
	// concurrently with each other. We delay the answer to the request for
	// the file data in an attempt to make sure the symlink syncs first, and
	// the file clobbers something on the other side of the symlink.
	//
	// What should actually happen to make the test pass is that we should
	// never make a request for the file data, because we should realize
	// that it is behind a symlink.

	if runtime.GOOS == "windows" {
		t.Skip("no symlink support on CI")
		return
	}

	defer os.RemoveAll("_tmpfolder")

	os.RemoveAll("_tmpfolder")
	if err := os.MkdirAll("_tmpfolder/dir", 0755); err != nil {
		t.Fatal(err)
	}

	m, fc := setupModelWithConnection()
	defer m.Stop()

	m.ScanFolder("default")

	// We listen for incoming index updates and trigger when we see one for
	// the expected names.
	dirDone := make(chan struct{})
	badReq := make(chan string, 1)
	badIdx := make(chan string, 1)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if f.Name == "dir" {
				close(dirDone)
				return
			}
			if strings.HasPrefix(f.Name, "dir") {
				badIdx <- f.Name
				return
			}
		}
	}
	fc.requestFn = func(folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error) {
		if name != "dir" && strings.HasPrefix(name, "dir") {
			// Make sure this request is a little bit slower.
			time.Sleep(250 * time.Millisecond)
			badReq <- name
		}
		return fc.fileData[name], nil
	}
	fc.mut.Unlock()

	// Send an update for the symlink to replace the directory, and a file
	// to add behind it. Expect an update for the "dir" (now symlink) but
	// not for the testfile.
	fc.addFile("dir", 0644, protocol.FileInfoTypeSymlinkDirectory, []byte(".."))
	fc.files[len(fc.files)-1].Version = fc.files[len(fc.files)-1].Version.Update(device1.Short())
	fc.addFile("dir/testfile", 0644, protocol.FileInfoTypeFile, []byte("datadata"))
	fc.sendIndexUpdate()
	<-dirDone

	select {
	case name := <-badReq:
		t.Fatal("Should not have requested the data for", name)
	case name := <-badIdx:
		t.Fatal("Should not have sent the index entry for", name)
	case <-time.After(3 * time.Second):
		// Unfortunately not much else to trigger on here. The puller sleep
		// interval is 1s so if we didn't get any requests within two
		// iterations we should be fine.
	}
}

func setupModelWithConnection() (*Model, *fakeConnection) {
	cfg := defaultConfig.RawCopy()
	cfg.Folders[0] = config.NewFolderConfiguration("default", "_tmpfolder")
	cfg.Folders[0].PullerSleepS = 1
	cfg.Folders[0].Devices = []config.FolderDeviceConfiguration{
		{DeviceID: device1},
		{DeviceID: device2},
	}
	w := config.Wrap("/tmp/cfg", cfg)

	db := db.OpenMemory()
	m := NewModel(w, device1, "device", "syncthing", "dev", db, nil)
	m.AddFolder(cfg.Folders[0])
	m.StartFolder("default")
	m.ServeBackground()

	fc := addFakeConn(m, device2)
	fc.folder = "default"

	return m, fc
}
