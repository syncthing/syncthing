// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

func TestRecvOnlyRevertDeletes(t *testing.T) {
	// Make sure that we delete extraneous files and directories when we hit
	// Revert.

	if err := os.RemoveAll("_recvonly"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll("_recvonly"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}()

	// Create some test data

	if err := os.MkdirAll("_recvonly/.stfolder", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("_recvonly/ignDir", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("_recvonly/unknownDir", 0755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("_recvonly/ignDir/ignFile", []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("_recvonly/unknownDir/unknownFile", []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("_recvonly/.stignore", []byte("ignDir\n"), 0644); err != nil {
		t.Fatal(err)
	}

	knownFiles := setupKnownFiles(t, []byte("hello\n"))

	// Get us a model up and running

	m := setupROFolder()
	defer m.Stop()

	// Send and index update for the known stuff

	m.Index(device1, "ro", knownFiles)
	m.updateLocalsFromScanning("ro", knownFiles)

	size := m.GlobalSize("ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}

	// Start the folder. This will cause a scan, should discover the other stuff in the folder

	m.StartFolder("ro")
	m.ScanFolder("ro")

	// We should now have two files and two directories.

	size = m.GlobalSize("ro")
	if size.Files != 2 || size.Directories != 2 {
		t.Fatalf("Global: expected 2 files and 2 directories: %+v", size)
	}
	size = m.LocalSize("ro")
	if size.Files != 2 || size.Directories != 2 {
		t.Fatalf("Local: expected 2 files and 2 directories: %+v", size)
	}
	size = m.ReceiveOnlyChangedSize("ro")
	if size.Files+size.Directories == 0 {
		t.Fatalf("ROChanged: expected something: %+v", size)
	}

	// Revert should delete the unknown stuff

	m.Revert("ro")

	// These should still exist
	for _, p := range []string{"_recvonly/knownDir/knownFile", "_recvonly/ignDir/ignFile"} {
		_, err := os.Stat(p)
		if err != nil {
			t.Error("Unexpected error:", err)
		}
	}

	// These should have been removed
	for _, p := range []string{"_recvonly/unknownDir", "_recvonly/unknownDir/unknownFile"} {
		_, err := os.Stat(p)
		if !os.IsNotExist(err) {
			t.Error("Unexpected existing thing:", p)
		}
	}

	// We should now have one file and directory again.

	size = m.GlobalSize("ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 files and 1 directories: %+v", size)
	}
	size = m.LocalSize("ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 files and 1 directories: %+v", size)
	}
}

func TestRecvOnlyRevertNeeds(t *testing.T) {
	// Make sure that a new file gets picked up and considered latest, then
	// gets considered old when we hit Revert.

	if err := os.RemoveAll("_recvonly"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll("_recvonly"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}()

	// Create some test data

	if err := os.MkdirAll("_recvonly/.stfolder", 0755); err != nil {
		t.Fatal(err)
	}
	oldData := []byte("hello\n")
	knownFiles := setupKnownFiles(t, oldData)

	// Get us a model up and running

	m := setupROFolder()
	defer m.Stop()

	// Send and index update for the known stuff

	m.Index(device1, "ro", knownFiles)
	m.updateLocalsFromScanning("ro", knownFiles)

	// Start the folder. This will cause a scan.

	m.StartFolder("ro")
	m.ScanFolder("ro")

	// Everything should be in sync.

	size := m.GlobalSize("ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}
	size = m.LocalSize("ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 file and 1 directory: %+v", size)
	}
	size = m.NeedSize("ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = m.ReceiveOnlyChangedSize("ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("ROChanged: expected nothing: %+v", size)
	}

	// Update the file.

	newData := []byte("totally different data\n")
	if err := ioutil.WriteFile("_recvonly/knownDir/knownFile", newData, 0644); err != nil {
		t.Fatal(err)
	}

	// Rescan.

	if err := m.ScanFolder("ro"); err != nil {
		t.Fatal(err)
	}

	// We now have a newer file than the rest of the cluster. Global state should reflect this.

	size = m.GlobalSize("ro")
	const sizeOfDir = 128
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(newData)) {
		t.Fatalf("Global: expected the new file to be reflected: %+v", size)
	}
	size = m.LocalSize("ro")
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(newData)) {
		t.Fatalf("Local: expected the new file to be reflected: %+v", size)
	}
	size = m.NeedSize("ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = m.ReceiveOnlyChangedSize("ro")
	if size.Files+size.Directories == 0 {
		t.Fatalf("ROChanged: expected something: %+v", size)
	}

	// We hit the Revert button. The file that was new should become old.

	m.Revert("ro")

	size = m.GlobalSize("ro")
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(oldData)) {
		t.Fatalf("Global: expected the global size to revert: %+v", size)
	}
	size = m.LocalSize("ro")
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(newData)) {
		t.Fatalf("Local: expected the local size to remain: %+v", size)
	}
	size = m.NeedSize("ro")
	if size.Files != 1 || size.Bytes != int64(len(oldData)) {
		t.Fatalf("Local: expected to need the old file data: %+v", size)
	}
}

func setupKnownFiles(t *testing.T, data []byte) []protocol.FileInfo {
	if err := os.MkdirAll("_recvonly/knownDir", 0755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("_recvonly/knownDir/knownFile", data, 0644); err != nil {
		t.Fatal(err)
	}

	t0 := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes("_recvonly/knownDir/knownFile", t0, t0); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat("_recvonly/knownDir/knownFile")
	if err != nil {
		t.Fatal(err)
	}
	blocks, _ := scanner.Blocks(context.TODO(), bytes.NewReader(data), protocol.BlockSize(int64(len(data))), int64(len(data)), nil, true)
	knownFiles := []protocol.FileInfo{
		{
			Name:        "knownDir",
			Type:        protocol.FileInfoTypeDirectory,
			Permissions: 0755,
			Version:     protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 42}}},
			Sequence:    42,
		},
		{
			Name:        "knownDir/knownFile",
			Type:        protocol.FileInfoTypeFile,
			Permissions: 0644,
			Size:        fi.Size(),
			ModifiedS:   fi.ModTime().Unix(),
			ModifiedNs:  int32(fi.ModTime().UnixNano() % 1e9),
			Version:     protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 42}}},
			Sequence:    42,
			Blocks:      blocks,
		},
	}

	return knownFiles
}

func setupROFolder() *Model {
	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, "ro", "receive only test", fs.FilesystemTypeBasic, "_recvonly")
	fcfg.Type = config.FolderTypeReceiveOnly
	fcfg.Devices = []config.FolderDeviceConfiguration{{DeviceID: device1}}
	fcfg.FSWatcherEnabled = false
	fcfg.RescanIntervalS = 86400

	cfg := defaultCfg.Copy()
	cfg.Folders = append(cfg.Folders, fcfg)

	wrp := config.Wrap("/dev/null", cfg)

	db := db.OpenMemory()
	m := NewModel(wrp, protocol.LocalDeviceID, "syncthing", "dev", db, nil)

	m.ServeBackground()
	m.AddFolder(fcfg)

	return m
}
