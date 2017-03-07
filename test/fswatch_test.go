// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"fmt"
	"log"
	"os"
	"testing"
)

func TestIgnoreOurOwnFsEvents(t *testing.T) {
	log.Println("Starting sender instance...")
	cleanUp(t, 1)
	sender := startInstance(t, 1)
	defer checkedStop(t, sender)

	log.Println("Starting receiver instance...")
	cleanUp(t, 2)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	log.Println("Creating directories and files...")
	dirs := []string{"d1", ".stversions"}
	files := []string{"d1/f1.TXT",
		".stignore",
		makeTempFilename("test")}
	all := append(files, dirs...)
	createDirectories(t, "s1", dirs)
	createFiles(t, "s1", files)

	waitForSync(t, sender)

	expected := len(all) - 3
	assertFileCount(t, sender, "default", expected)

	assertFileCount(t, receiver, "default", expected)
}

func cleanUp(t *testing.T, index int) {
	log.Println("Cleaning up test files/directories...")
	err := removeAll(fmt.Sprintf("s%d", index),
		fmt.Sprintf("h%d/index*", index))
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir(fmt.Sprintf("s%d", index), 0755)
	if err != nil {
		t.Fatal(err)
	}
}
