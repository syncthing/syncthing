// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestRecvOnlyRevertDeletes(t *testing.T) {
	os.RemoveAll("_recvonly")
	defer os.RemoveAll("_recvonly")

	// Create some test data

	os.MkdirAll("_recvonly/.stfolder", 0755)
	os.MkdirAll("_recvonly/knownDir", 0755)
	os.MkdirAll("_recvonly/ignDir", 0755)
	os.MkdirAll("_recvonly/unknownDir", 0755)
	ioutil.WriteFile("_recvonly/knownDir/knownFile", []byte("hello\n"), 0644)
	ioutil.WriteFile("_recvonly/ignDir/ignFile", []byte("hello\n"), 0644)
	ioutil.WriteFile("_recvonly/unknownDir/unknownFile", []byte("hello\n"), 0644)
	ioutil.WriteFile("_recvonly/.stignore", []byte("ignDir\n"), 0644)

	// Get us a model up and running

	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, "ro", "receive only test", fs.FilesystemTypeBasic, "_recvonly")
	fcfg.Type = config.FolderTypeReceiveOnly
	fcfg.Devices = []config.FolderDeviceConfiguration{{DeviceID: device1}}

	cfg := defaultCfg.Copy()
	cfg.Folders = append(cfg.Folders, fcfg)

	wrp := config.Wrap("/dev/null", cfg)

	db := db.OpenMemory()
	m := NewModel(wrp, protocol.LocalDeviceID, "syncthing", "dev", db, nil)

	m.ServeBackground()
	defer m.Stop()

	// Add the folder and send and index update for the known stuff

	fi, err := os.Stat("_recvonly/knownDir/knownFile")
	if err != nil {
		t.Fatal(err)
	}
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
		},
	}

	m.AddFolder(fcfg)
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
