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
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/model"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func TestFolderErrors(t *testing.T) {
	fcfg := config.FolderConfiguration{
		ID:      "folder",
		RawPath: "testdata/testfolder",
	}
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	for _, file := range []string{".stfolder", "testfolder/.stfolder", "testfolder"} {
		os.Remove("testdata/" + file)
		_, err := os.Stat("testdata/" + file)
		if err == nil {
			t.Error("Found unexpected file")
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

	os.Remove("testdata/testfolder/.stfolder")
	os.Remove("testdata/testfolder/")

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

	os.Remove("testdata/.stfolder")

	// Case 3 - Folder marker missing

	set := db.NewFileSet("folder", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	m = model.NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)
	m.AddFolder(fcfg)

	if err := m.CheckFolderHealth("folder"); err == nil || err.Error() != "Folder marker missing" {
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

	os.Remove("testdata/testfolder/.stfolder")
	os.Remove("testdata/testfolder/")

	fcfg.RawPath = "testdata/testfolder"
	cfg = config.Wrap("testdata/subfolder", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	m = model.NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)
	m.AddFolder(fcfg)

	if err := m.CheckFolderHealth("folder"); err == nil || err.Error() != "Folder path missing" {
		t.Error("Incorrect error: Folder path missing !=", m.CheckFolderHealth("folder"))
	}

	// Case 4.1 - recover after folder path missing

	os.Mkdir("testdata/testfolder", 0700)

	if err := m.CheckFolderHealth("folder"); err == nil || err.Error() != "Folder marker missing" {
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
