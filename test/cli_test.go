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

package integration_test

import (
	"os"
	"os/exec"
	"testing"
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
}
