// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIGenerate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// --generate should create a bunch of stuff

	cmd := exec.Command("../bin/syncthing", "--no-browser", "--generate", dir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Verify that the files that should have been created have been

	for _, f := range []string{"config.xml", "cert.pem", "key.pem"} {
		_, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Errorf("%s is not correctly generated", f)
		}
	}
}

func TestCLIFirstStartup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// First startup should create config, BEP certificate, and HTTP certificate.

	cmd := exec.Command("../bin/syncthing", "--no-browser", "--home", dir)
	cmd.Env = append(os.Environ(), "STNORESTART=1")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
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
				for _, f := range []string{"config.xml", "cert.pem", "key.pem", "https-cert.pem", "https-key.pem"} {
					_, err := os.Stat(filepath.Join(dir, f))
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
