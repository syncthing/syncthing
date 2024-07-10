// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/syncthing/syncthing/lib/rc"
)

const indexDbDir = "index-v0.14.0.db"

var generatedFiles = []string{"config.xml", "cert.pem", "key.pem"}

func TestCLIReset(t *testing.T) {
	t.Parallel()

	instance := startInstance(t)

	// Shutdown instance after it created its files in syncthing's home directory.
	api := rc.NewAPI(instance.apiAddress, instance.apiKey)
	err := api.Post("/rest/system/shutdown", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	dbDir := filepath.Join(instance.syncthingDir, indexDbDir)
	err = os.MkdirAll(dbDir, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(syncthingBinary, "--no-browser", "--no-default-folder", "--home", instance.syncthingDir, "--reset-database")
	cmd.Env = basicEnv(instance.userHomeDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(dbDir)
	if err == nil {
		t.Errorf("the directory %q still exists, expected it to have been deleted", dbDir)
	}
}

func TestCLIGenerate(t *testing.T) {
	t.Parallel()

	syncthingDir := t.TempDir()
	userHomeDir := t.TempDir()
	generateDir := t.TempDir()

	cmd := exec.Command(syncthingBinary, "--no-browser", "--no-default-folder", "--home", syncthingDir, "--generate", generateDir)
	cmd.Env = basicEnv(userHomeDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	found := walk(t, generateDir)
	// Sort list so binary search works.
	sort.Strings(found)

	// Verify that the files that should have been created have been.
	for _, want := range generatedFiles {
		_, ok := slices.BinarySearch(found, want)
		if !ok {
			t.Errorf("expected to find %q in %q", want, generateDir)
		}
	}
}

func TestCLIFirstStartup(t *testing.T) {
	t.Parallel()

	// Startup instance.
	instance := startInstance(t)

	// Shutdown instance after it created its files in syncthing's home directory.
	api := rc.NewAPI(instance.apiAddress, instance.apiKey)
	err := api.Post("/rest/system/shutdown", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	found := walk(t, instance.syncthingDir)

	// Sort list so binary search works.
	sort.Strings(found)

	// Verify that the files that should have been created have been.
	for _, want := range generatedFiles {
		_, ok := slices.BinarySearch(found, want)
		if !ok {
			t.Errorf("expected to find %q in %q", want, instance.syncthingDir)
		}
	}
}
