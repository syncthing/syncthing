// Copyright (C) 2014 The Syncthing Authors.
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

// +build integration

package integration

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
)

func TestFileTypeChange(t *testing.T) {
	// Use no versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{}
	cfg.SetFolder(fld)
	cfg.Save()

	testFileTypeChange(t)
}

func TestFileTypeChangeSimpleVersioning(t *testing.T) {
	// Use simple versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type:   "simple",
		Params: map[string]string{"keep": "5"},
	}
	cfg.SetFolder(fld)
	cfg.Save()

	testFileTypeChange(t)
}

func TestFileTypeChangeStaggeredVersioning(t *testing.T) {
	// Use staggered versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type: "staggered",
	}
	cfg.SetFolder(fld)
	cfg.Save()

	testFileTypeChange(t)
}

func testFileTypeChange(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 100, 20, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// A file that we will replace with a directory later

	fd, err := os.Create("s1/fileToReplace")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// A directory that we will replace with a file later

	err = os.Mkdir("s1/dirToReplace", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the files and directories sync to the other side

	log.Println("Syncing...")

	sender := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	receiver := syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = receiver.start()
	if err != nil {
		_ = sender.stop()
		t.Fatal(err)
	}

	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			if isTimeout(err) {
				time.Sleep(time.Second)
				continue
			}
			_ = sender.stop()
			_ = receiver.stop()
			t.Fatal(err)
		}

		curComp := comp[id2]

		if curComp == 100 {
			_ = sender.stop()
			_ = receiver.stop()
			break
		}

		time.Sleep(time.Second)
	}

	err = sender.stop()
	if err != nil {
		t.Fatal(err)
	}
	err = receiver.stop()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Making some changes...")

	// Replace file with directory

	os.RemoveAll("s1/fileToReplace")
	err = os.Mkdir("s1/fileToReplace", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Replace directory with file

	os.RemoveAll("s1/dirToReplace")
	fd, err = os.Create("s1/dirToReplace")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// Sync these changes and recheck

	log.Println("Syncing...")

	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	err = receiver.start()
	if err != nil {
		_ = sender.stop()
		t.Fatal(err)
	}

	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			if isTimeout(err) {
				time.Sleep(time.Second)
				continue
			}
			_ = sender.stop()
			_ = receiver.stop()
			t.Fatal(err)
		}

		curComp := comp[id2]

		if curComp == 100 {
			break
		}

		time.Sleep(time.Second)
	}

	err = sender.stop()
	if err != nil {
		t.Fatal(err)
	}
	err = receiver.stop()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
