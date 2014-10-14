// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/files"
	"github.com/syncthing/syncthing/internal/model"
	"github.com/syncthing/syncthing/internal/protocol"

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

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)

	// Case 1 - new folder, directory and marker created

	m := model.NewModel(cfg, "device", "syncthing", "dev", db)
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

	m = model.NewModel(cfg, "device", "syncthing", "dev", db)
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

	set := files.NewSet("folder", db)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	m = model.NewModel(cfg, "device", "syncthing", "dev", db)
	sanityCheckFolders(cfg, m)

	if cfg.Folders()["folder"].Invalid != "folder marker missing" {
		t.Error("Incorrect error")
	}

	// Case 4 - path missing

	fcfg.Path = "testdata/testfolder"
	cfg = config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	})

	m = model.NewModel(cfg, "device", "syncthing", "dev", db)
	sanityCheckFolders(cfg, m)

	if cfg.Folders()["folder"].Invalid != "folder path missing" {
		t.Error("Incorrect error")
	}
}
