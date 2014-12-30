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
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/protocol"
)

func TestSyncCluster(t *testing.T) {
	// Use no versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{}
	cfg.SetFolder(fld)
	cfg.Save()

	testSyncCluster(t)
}

func TestSyncClusterSimpleVersioning(t *testing.T) {
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

	testSyncCluster(t)
}

func TestSyncClusterStaggeredVersioning(t *testing.T) {
	// Use staggered versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type: "staggered",
	}
	cfg.SetFolder(fld)
	cfg.Save()

	testSyncCluster(t)
}

func testSyncCluster(t *testing.T) {
	/*

		This tests syncing files back and forth between three cluster members.
		Their configs are in h1, h2 and h3. The folder "default" is shared
		between all and stored in s1, s2 and s3 respectively.

		Another folder is shared between 1 and 2 only, in s12-1 and s12-2. A
		third folders is shared between 2 and 3, in s23-2 and s23-3.

	*/
	log.Println("Cleaning...")
	err := removeAll("s1", "s12-1",
		"s2", "s12-2", "s23-2",
		"s3", "s23-3",
		"h1/index", "h2/index", "h3/index")
	if err != nil {
		t.Fatal(err)
	}

	// Create initial folder contents. All three devices have stuff in
	// "default", which should be merged. The other two folders are initially
	// empty on one side.

	log.Println("Generating files...")

	err = generateFiles("s1", 1000, 21, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	err = generateFiles("s12-1", 1000, 21, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// We'll use this file for appending data without modifying the time stamp.
	fd, err := os.Create("s1/appendfile")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("hello\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = generateFiles("s2", 1000, 21, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	err = generateFiles("s23-2", 1000, 21, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	err = generateFiles("s3", 1000, 21, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	p, err := scStartProcesses()
	if err != nil {
		t.Fatal(err)
	}

	// Prepare the expected state of folders after the sync
	c1, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	c3, err := directoryContents("s3")
	if err != nil {
		t.Fatal(err)
	}
	e1 := mergeDirectoryContents(c1, c2, c3)
	e2, err := directoryContents("s12-1")
	if err != nil {
		t.Fatal(err)
	}
	e3, err := directoryContents("s23-2")
	if err != nil {
		t.Fatal(err)
	}
	expected := [][]fileInfo{e1, e2, e3}

	for count := 0; count < 5; count++ {
		log.Println("Forcing rescan...")

		// Force rescan of folders
		for i := range p {
			p[i].post("/rest/scan?folder=default", nil)
			if i < 3 {
				p[i].post("/rest/scan?folder=s12", nil)
			}
			if i > 1 {
				p[i].post("/rest/scan?folder=s23", nil)
			}
		}

		// Sync stuff and verify it looks right
		err = scSyncAndCompare(p, expected)
		if err != nil {
			t.Error(err)
			break
		}

		log.Println("Altering...")

		// Alter the source files for another round
		err = alterFiles("s1")
		if err != nil {
			t.Error(err)
			break
		}
		err = alterFiles("s12-1")
		if err != nil {
			t.Error(err)
			break
		}
		err = alterFiles("s23-2")
		if err != nil {
			t.Error(err)
			break
		}

		// Alter the "appendfile" without changing it's modification time. Sneaky!
		fi, err := os.Stat("s1/appendfile")
		if err != nil {
			t.Fatal(err)
		}
		fd, err := os.OpenFile("s1/appendfile", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fd.Seek(0, os.SEEK_END)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fd.WriteString("more data\n")
		if err != nil {
			t.Fatal(err)
		}
		err = fd.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = os.Chtimes("s1/appendfile", fi.ModTime(), fi.ModTime())
		if err != nil {
			t.Fatal(err)
		}

		// Prepare the expected state of folders after the sync
		e1, err = directoryContents("s1")
		if err != nil {
			t.Fatal(err)
		}
		e2, err = directoryContents("s12-1")
		if err != nil {
			t.Fatal(err)
		}
		e3, err = directoryContents("s23-2")
		if err != nil {
			t.Fatal(err)
		}
		expected = [][]fileInfo{e1, e2, e3}
	}

	for i := range p {
		p[i].stop()
	}
}

func scStartProcesses() ([]syncthingProcess, error) {
	p := make([]syncthingProcess, 3)

	p[0] = syncthingProcess{ // id1
		log:    "1.out",
		argv:   []string{"-home", "h1"},
		port:   8081,
		apiKey: apiKey,
	}
	err := p[0].start()
	if err != nil {
		return nil, err
	}

	p[1] = syncthingProcess{ // id2
		log:    "2.out",
		argv:   []string{"-home", "h2"},
		port:   8082,
		apiKey: apiKey,
	}
	err = p[1].start()
	if err != nil {
		_ = p[0].stop()
		return nil, err
	}

	p[2] = syncthingProcess{ // id3
		log:    "3.out",
		argv:   []string{"-home", "h3"},
		port:   8083,
		apiKey: apiKey,
	}
	err = p[2].start()
	if err != nil {
		_ = p[0].stop()
		_ = p[1].stop()
		return nil, err
	}

	return p, nil
}

func scSyncAndCompare(p []syncthingProcess, expected [][]fileInfo) error {
	ids := []string{id1, id2, id3}

	log.Println("Syncing...")

mainLoop:
	for {
		time.Sleep(2500 * time.Millisecond)

		for i := range p {
			comp, err := p[i].peerCompletion()
			if err != nil {
				if isTimeout(err) {
					continue mainLoop
				}
				return err
			}

			for id, pct := range comp {
				if id == ids[i] {
					// Don't check for self, which will be 0%
					continue
				}
				if pct != 100 {
					log.Printf("%s not done yet: %d%%", id, pct)
					continue mainLoop
				}
			}
		}

		break
	}

	log.Println("Checking...")

	for _, dir := range []string{"s1", "s2", "s3"} {
		actual, err := directoryContents(dir)
		if err != nil {
			return err
		}
		if err := compareDirectoryContents(actual, expected[0]); err != nil {
			return fmt.Errorf("%s: %v", dir, err)
		}
	}

	for _, dir := range []string{"s12-1", "s12-2"} {
		actual, err := directoryContents(dir)
		if err != nil {
			return err
		}
		if err := compareDirectoryContents(actual, expected[1]); err != nil {
			return fmt.Errorf("%s: %v", dir, err)
		}
	}

	for _, dir := range []string{"s23-2", "s23-3"} {
		actual, err := directoryContents(dir)
		if err != nil {
			return err
		}
		if err := compareDirectoryContents(actual, expected[2]); err != nil {
			return fmt.Errorf("%s: %v", dir, err)
		}
	}

	return nil
}
