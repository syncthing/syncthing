// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"testing"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/model"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func TestFolderErrors(t *testing.T) {
	// This test intentionally avoids starting the folders. If they are
	// started, they will perform an initial scan, which will create missing
	// folder markers and race with the stuff we do in the test.

	fcfg := config.FolderConfiguration{
		ID:      "folder",
		RawPath: "testdata/testfolder",
	}
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	for _, file := range []string{".stfolder", "testfolder/.stfolder", "testfolder"} {
		if err := os.Remove("testdata/" + file); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	ldb, _ := leveldb.Open(storage.NewMemStorage(), nil)

	// Case 1 - new folder, directory and marker created

	m := model.NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)
	m.AddFolder(fcfg)

	if err := m.CheckFolderHealth("folder"); err != nil {
		t.Error("Unexpected error", cfg.Folders()["folder"].Invalid)
	}

	s, err := os.Stat("testdata/testfolder")
	if err != nil || !s.IsDir() {
		t.Error(err)
	}

	_, err = os.Stat("testdata/testfolder/.stfolder")
	if err != nil {
		t.Error(err)
	}

	if err := os.Remove("testdata/testfolder/.stfolder"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("testdata/testfolder/"); err != nil {
		t.Fatal(err)
	}

	// Case 2 - new folder, marker created

	fcfg.RawPath = "testdata/"
	cfg = config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	m = model.NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)
	m.AddFolder(fcfg)

	if err := m.CheckFolderHealth("folder"); err != nil {
		t.Error("Unexpected error", cfg.Folders()["folder"].Invalid)
	}

	_, err = os.Stat("testdata/.stfolder")
	if err != nil {
		t.Error(err)
	}

	if err := os.Remove("testdata/.stfolder"); err != nil {
		t.Fatal(err)
	}

	// Case 3 - Folder marker missing

	set := db.NewFileSet("folder", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	m = model.NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)
	m.AddFolder(fcfg)

	if err := m.CheckFolderHealth("folder"); err == nil || err.Error() != "folder marker missing" {
		t.Error("Incorrect error: Folder marker missing !=", m.CheckFolderHealth("folder"))
	}

	// Case 3.1 - recover after folder marker missing

	if err = fcfg.CreateMarker(); err != nil {
		t.Error(err)
	}

	if err := m.CheckFolderHealth("folder"); err != nil {
		t.Error("Unexpected error", cfg.Folders()["folder"].Invalid)
	}

	// Case 4 - Folder path missing

	if err := os.Remove("testdata/testfolder/.stfolder"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Remove("testdata/testfolder"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	fcfg.RawPath = "testdata/testfolder"
	cfg = config.Wrap("testdata/subfolder", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	m = model.NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)
	m.AddFolder(fcfg)

	if err := m.CheckFolderHealth("folder"); err == nil || err.Error() != "folder path missing" {
		t.Error("Incorrect error: Folder path missing !=", m.CheckFolderHealth("folder"))
	}

	// Case 4.1 - recover after folder path missing

	if err := os.Mkdir("testdata/testfolder", 0700); err != nil {
		t.Fatal(err)
	}

	if err := m.CheckFolderHealth("folder"); err == nil || err.Error() != "folder marker missing" {
		t.Error("Incorrect error: Folder marker missing !=", m.CheckFolderHealth("folder"))
	}

	// Case 4.2 - recover after missing marker

	if err = fcfg.CreateMarker(); err != nil {
		t.Error(err)
	}

	if err := m.CheckFolderHealth("folder"); err != nil {
		t.Error("Unexpected error", cfg.Folders()["folder"].Invalid)
	}
}

func TestShortIDCheck(t *testing.T) {
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 0, 0}},
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 1, 1}}, // first 56 bits same, differ in the first 64 bits
		},
	})

	if err := checkShortIDs(cfg); err != nil {
		t.Error("Unexpected error:", err)
	}

	cfg = config.Wrap("/tmp/test", config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 64, 0}},
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 64, 1}}, // first 64 bits same
		},
	})

	if err := checkShortIDs(cfg); err == nil {
		t.Error("Should have gotten an error")
	}
}
