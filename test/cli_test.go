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
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIReset(t *testing.T) {
	dirs := []string{"s1", "s12-1", "h1/index"}

	// Create directories that reset will remove

	for _, dir := range dirs {
		err := os.Mkdir(dir, 0755)
		if err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}

	// Run reset to clean up

	cmd := exec.Command("../bin/syncthing", "-home", "h1", "-reset")
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

	// -generate should create a bunch of stuff

	cmd := exec.Command("../bin/syncthing", "-generate", "home.out")
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

	cmd := exec.Command("../bin/syncthing", "-home", "home.out")
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
