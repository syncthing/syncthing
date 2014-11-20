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

// This currently fails; it should be fixed
package integration_test_disabled

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFiletypeChange(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 100, 20, "../bin/syncthing")
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
		log:    "1.out",
		argv:   []string{"-home", "h1"},
		port:   8081,
		apiKey: apiKey,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	receiver := syncthingProcess{ // id2
		log:    "2.out",
		argv:   []string{"-home", "h2"},
		port:   8082,
		apiKey: apiKey,
	}
	err = receiver.start()
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}

	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				time.Sleep(time.Second)
				continue
			}
			sender.stop()
			receiver.stop()
			t.Fatal(err)
		}

		curComp := comp[id2]

		if curComp == 100 {
			sender.stop()
			receiver.stop()
			break
		}

		time.Sleep(time.Second)
	}

	sender.stop()
	receiver.stop()

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
		sender.stop()
		t.Fatal(err)
	}

	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				time.Sleep(time.Second)
				continue
			}
			sender.stop()
			receiver.stop()
			t.Fatal(err)
		}

		curComp := comp[id2]

		if curComp == 100 {
			sender.stop()
			receiver.stop()
			break
		}

		time.Sleep(time.Second)
	}

	sender.stop()
	receiver.stop()

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
