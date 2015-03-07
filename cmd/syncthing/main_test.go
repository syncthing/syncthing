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

func TestSanityCheck(t *testing.T) {
	fcfg := config.FolderConfiguration{
		ID:   "folder",
		Path: "testdata/testfolder",
	}
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	for _, file := range []string{".stfolder", "testfolder", "testfolder/.stfolder"} {
		_, err := os.Stat("testdata/" + file)
		if err == nil {
			t.Error("Found unexpected file")
		}
	}

	ldb, _ := leveldb.Open(storage.NewMemStorage(), nil)

	// Case 1 - new folder, directory and marker created

	m := model.NewModel(cfg, "device", "syncthing", "dev", ldb)
	sanityCheckFolders(cfg, m)

	if cfg.Folders()["folder"].Invalid != "" {
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

	fcfg.Path = "testdata/"
	cfg = config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	m = model.NewModel(cfg, "device", "syncthing", "dev", ldb)
	sanityCheckFolders(cfg, m)

	if cfg.Folders()["folder"].Invalid != "" {
		t.Error("Unexpected error", cfg.Folders()["folder"].Invalid)
	}

	_, err = os.Stat("testdata/.stfolder")
	if err != nil {
		t.Error(err)
	}

	os.Remove("testdata/.stfolder")

	// Case 3 - marker missing

	set := db.NewFileSet("folder", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	m = model.NewModel(cfg, "device", "syncthing", "dev", ldb)
	sanityCheckFolders(cfg, m)

	if cfg.Folders()["folder"].Invalid != "folder marker missing" {
		t.Error("Incorrect error")
	}

	// Case 4 - path missing

	fcfg.Path = "testdata/testfolder"
	cfg = config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	m = model.NewModel(cfg, "device", "syncthing", "dev", ldb)
	sanityCheckFolders(cfg, m)

	if cfg.Folders()["folder"].Invalid != "folder path missing" {
		t.Error("Incorrect error")
	}
}
