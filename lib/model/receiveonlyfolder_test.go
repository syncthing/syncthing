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
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestReceiveOnlyFileNewGlobalVersion(t *testing.T) {
	// Verify that a locally modified file gets replaced by the global version
	// after the global version gets changed

	os.RemoveAll("_tmpfolder")

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnectionReceiveOnly(false, false)
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if strings.Contains(f.Name, "testfile") {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	remotecontent := []byte("remote content\n")
	newremotecontent := []byte("updated remote  content\n")

	// Send an update for the test file, wait for it to sync and be reported back.
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, remotecontent)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err := ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not sync correctly:", err)
		return
	}
	if !bytes.Equal(bs, remotecontent) {
		t.Error("File did not sync correctly: incorrect data")
		return
	}

	// Create a newer version of the file in the cluster
	fc.updateFile("testfile", 0644, protocol.FileInfoTypeFile, newremotecontent, false)
	done = make(chan struct{})
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err = ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not resync correctly:", err)
		return
	}
	if !bytes.Equal(bs, newremotecontent) {
		t.Error("File did not resync correctly: incorrect data")
	}
}

func TestReceiveOnlyFileModifiedOverwriteFromCluster(t *testing.T) {
	// Verify that a locally modified file gets replaced by the global version
	// after the global version gets changed

	os.RemoveAll("_tmpfolder")

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnectionReceiveOnly(false, false)
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if strings.Contains(f.Name, "testfile") {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	remotecontent := []byte("remote content\n")
	newremotecontent := []byte("updated remote  content\n")
	localcontent := []byte("local change\n")

	// Send an update for the test file, wait for it to sync and be reported back.
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, remotecontent)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err := ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not sync correctly:", err)
		return
	}
	if !bytes.Equal(bs, remotecontent) {
		t.Error("File did not sync correctly: incorrect data")
		return
	}

	// Overwrite the contents of the file locally
	if err = ioutil.WriteFile("_tmpfolder/testfile", []byte(localcontent), 0644); err != nil {
		t.Fatal(err)
	}
	done = make(chan struct{})
	m.ScanFolder("default")
	// let's give the puller 2 iterations
	time.Sleep(2 * time.Second)

	// Verify the contents. File should still be the local version
	bs, err = ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not resync correctly:", err)
		return
	}
	if !bytes.Equal(bs, localcontent) {
		t.Error("File was downloaded from cluster again and should not have been")
		return
	}

	// Create a newer version of the file in the cluster
	fc.updateFile("testfile", 0644, protocol.FileInfoTypeFile, newremotecontent, false)
	done = make(chan struct{})
	m.ScanFolder("default")
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err = ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not resync correctly:", err)
		return
	}
	if !bytes.Equal(bs, newremotecontent) {
		t.Error("File did not resync correctly: incorrect data")
	}
}

func TestReceiveOnlyFileModifiedRevertLocalChange(t *testing.T) {
	// Verify that a locally modified file gets replaced by the global version

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnectionReceiveOnly(true, false)
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if strings.Contains(f.Name, "testfile") {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	goodcontent := []byte("test file contents\n")
	badcontent := []byte("unauthorized local change\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, goodcontent)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err := ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not sync correctly:", err)
		return
	}
	if !bytes.Equal(bs, goodcontent) {
		t.Error("File did not sync correctly: incorrect data")
		return
	}

	// Overwrite the contents of the file
	if err = ioutil.WriteFile("_tmpfolder/testfile", []byte(badcontent), 0644); err != nil {
		t.Fatal(err)
	}

	done = make(chan struct{})
	m.ScanFolder("default")
	<-done

	// Verify the contents
	bs, err = ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not resync correctly:", err)
		return
	}
	if !bytes.Equal(bs, goodcontent) {
		t.Error("File did not resync correctly: incorrect data")
	}
}

func TestReceiveOnlyFileModifiedDoNotRevertLocalChange(t *testing.T) {
	// Verify that a locally modified file gets replaced by the global version

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnectionReceiveOnly(false, false)
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if strings.Contains(f.Name, "testfile") {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	localcontent := []byte("test file contents\n")
	globalcontent := []byte("unauthorized local change\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, globalcontent)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err := ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not sync correctly:", err)
		return
	}
	if !bytes.Equal(bs, globalcontent) {
		t.Error("File did not sync correctly: incorrect data")
		return
	}

	// Overwrite the contents of the file
	if err = ioutil.WriteFile("_tmpfolder/testfile", []byte(localcontent), 0644); err != nil {
		t.Fatal(err)
	}

	// add another test file, otherwise we'll get an endless loop.
	fc.addFile("testfile2", 0644, protocol.FileInfoTypeFile, globalcontent)
	fc.sendIndexUpdate()

	done = make(chan struct{})
	m.ScanFolder("default")
	<-done

	fc.sendIndexUpdate()

	// Verify the contents
	bs, err = ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("Error:", err)
		return
	}
	if !bytes.Equal(bs, localcontent) {
		t.Error("File was re-synced from cluster and should not have been")
	}
}

func TestReceiveOnlyFileDeletedResync(t *testing.T) {
	// Verify that a locally modified file gets replaced by the global version

	defer os.RemoveAll("_tmpfolder")

	m, fc := setupModelWithConnectionReceiveOnly(true, false)
	defer m.Stop()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if strings.Contains(f.Name, "testfile") {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	goodcontent := []byte("test file contents\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, goodcontent)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	bs, err := ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not sync correctly:", err)
		return
	}
	if !bytes.Equal(bs, goodcontent) {
		t.Error("File did not sync correctly: incorrect data")
		return
	}

	// Delete the file
	if err := os.Remove("_tmpfolder/testfile"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	done = make(chan struct{})
	m.ScanFolder("default")
	<-done

	// Verify the contents
	bs, err = ioutil.ReadFile("_tmpfolder/testfile")
	if err != nil {
		t.Error("File did not resync correctly:", err)
		return
	}
	if !bytes.Equal(bs, goodcontent) {
		t.Error("File did not resync correctly: incorrect data")
	}
}

func TestReceiveOnlyFileAddedAndRemoved(t *testing.T) {
	// Verify that a locally added file gets removed

	defer os.RemoveAll("_tmpfolder")

	m, _ := setupModelWithConnectionReceiveOnly(true, true)
	defer m.Stop()

	badcontent := []byte("unauthorized local change\n")

	// Add an unwanted local file
	if err := ioutil.WriteFile("_tmpfolder/testfile", []byte(badcontent), 0644); err != nil {
		t.Fatal(err)
	}

	m.ScanFolder("default")

	if _, err := os.Stat("_tmpfolder/testfile"); !os.IsNotExist(err) {
		t.Error("Locally rejected file was not removed")
	}
}

func TestReceiveOnlyFileAddedAndNotRemoved(t *testing.T) {
	// Verify that a locally added file gets NOT removed

	defer os.RemoveAll("_tmpfolder")

	m, _ := setupModelWithConnectionReceiveOnly(true, false)
	defer m.Stop()

	badcontent := []byte("unauthorized local change\n")

	// Add an unwanted local file
	if err := ioutil.WriteFile("_tmpfolder/testfile", []byte(badcontent), 0644); err != nil {
		t.Fatal(err)
	}

	m.ScanFolder("default")

	if _, err := os.Stat("_tmpfolder/testfile"); os.IsNotExist(err) {
		t.Error("Locally rejected file was removed and should have remained")
	}
}

func setupModelWithConnectionReceiveOnly(revertLocalChanges bool, deleteLocalChanges bool) (*Model, *fakeConnection) {
	cfg := defaultConfig.RawCopy()
	cfg.Folders[0] = config.NewFolderConfiguration("default", "_tmpfolder")
	cfg.Folders[0].PullerSleepS = 1
	cfg.Folders[0].Type = config.FolderTypeReceiveOnly
	cfg.Folders[0].RevertLocalChanges = revertLocalChanges
	cfg.Folders[0].DeleteLocalChanges = deleteLocalChanges
	cfg.Folders[0].Devices = []config.FolderDeviceConfiguration{
		{DeviceID: device1},
		{DeviceID: device2},
	}
	w := config.Wrap("/tmp/cfg", cfg)

	db := db.OpenMemory()
	m := NewModel(w, device1, "device", "syncthing", "dev", db, nil)
	m.AddFolder(cfg.Folders[0])
	m.ServeBackground()
	m.StartFolder("default")

	fc := addFakeConn(m, device2)
	fc.folder = "default"

	return m, fc
}
