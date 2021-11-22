// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIReset(t *testing.T) {
	dirs := []string{"h1/index-v0.14.0.db"}

	// Create directories that reset will remove

	for _, dir := range dirs {
		err := os.Mkdir(dir, 0755)
		if err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}

	// Run reset to clean up

	cmd := exec.Command("../bin/syncthing", "--no-browser", "--home", "h1", "--reset-database")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	// Verify that they're gone

	for _, dir := range dirs {
		_, err := os.Stat(dir)
		if err == nil {
			t.Errorf("%s still exists", dir)
		}
	}

	// Clean up

	dirs, err = filepath.Glob("*.syncthing-reset-*")
	if err != nil {
		t.Fatal(err)
	}
	removeAll(dirs...)
}

func TestCLIGenerate(t *testing.T) {
	err := os.RemoveAll("home.out")
	if err != nil {
		t.Fatal(err)
	}

	// --generate should create a bunch of stuff

	cmd := exec.Command("../bin/syncthing", "--no-browser", "--generate", "home.out")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the files that should have been created have been

	for _, f := range []string{"home.out/config.xml", "home.out/cert.pem", "home.out/key.pem"} {
		_, err := os.Stat(f)
		if err != nil {
			t.Errorf("%s is not correctly generated", f)
		}
	}
}

func TestCLIFirstStartup(t *testing.T) {
	err := os.RemoveAll("home.out")
	if err != nil {
		t.Fatal(err)
	}

	// First startup should create config, BEP certificate, and HTTP certificate.

	cmd := exec.Command("../bin/syncthing", "--no-browser", "--home", "home.out")
	cmd.Env = append(os.Environ(), "STNORESTART=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	exitError := make(chan error, 1)
	filesOk := make(chan struct{})
	processDone := make(chan struct{})

	go func() {
		// Wait for process exit.
		exitError <- cmd.Wait()
		close(processDone)
	}()

	go func() {
	again:
		for {
			select {
			case <-processDone:
				return
			default:
				// Verify that the files that should have been created have been
				for _, f := range []string{"home.out/config.xml", "home.out/cert.pem", "home.out/key.pem", "home.out/https-cert.pem", "home.out/https-key.pem"} {
					_, err := os.Stat(f)
					if err != nil {
						time.Sleep(500 * time.Millisecond)
						continue again
					}
				}

				// Make sure the process doesn't exit with an error just after creating certificates.
				time.Sleep(time.Second)
				filesOk <- struct{}{}
				return
			}
		}
	}()

	select {
	case e := <-exitError:
		t.Error(e)
	case <-filesOk:
		cmd.Process.Kill()
		return
	}
}
