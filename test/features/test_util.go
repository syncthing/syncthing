// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.
package features

import (
	"io/ioutil"
	"os"
)

// TemporaryDirectoryForTests helps to run tests in a temporary directory
// while not messing up the current working directory afterwards
type TemporaryDirectoryForTests struct {
	Cwd           string
	testDirectory string
}

// Init saves current working directory for later recovery
func (t *TemporaryDirectoryForTests) Init() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	t.Cwd = cwd
}

// Setup creates and changes to temporary test directory
func (t *TemporaryDirectoryForTests) Setup() {
	if len(t.testDirectory) > 0 {
		panic("testDirectory is already set: " + t.testDirectory)
	}

	path, err := ioutil.TempDir("", "x")
	if err != nil {
		panic(err)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}

	if err := os.Chdir(path); err != nil {
		panic(err)
	}
	t.testDirectory = path
}

// Cleanup reset the current working directory and cleanup the temporary test directory
func (t *TemporaryDirectoryForTests) Cleanup() {
	if _cwd, err := os.Getwd(); err != nil {
		panic(err)
	} else {
		if len(t.testDirectory) > 0 && _cwd != t.Cwd {
			if err := os.Chdir(t.Cwd); err != nil {
				panic(err)
			}
			if err := os.RemoveAll(t.testDirectory); err != nil {
				panic(err)
			}
			t.testDirectory = ""
		}
	}
}
