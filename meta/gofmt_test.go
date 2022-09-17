// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package meta

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var gofmtCheckDirs = []string{".", "../cmd", "../lib", "../test", "../script"}

// Checks that files are properly gofmt:ed.
func TestCheckGoFmt(t *testing.T) {
	for _, dir := range gofmtCheckDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path == ".git" {
				return filepath.SkipDir
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, ".pb.go") {
				return nil
			}
			cmd := exec.Command("gofmt", "-s", "-d", path)
			bs, err := cmd.CombinedOutput()
			if err != nil {
				return err
			}
			if len(bs) != 0 {
				t.Errorf("File %s is not formatted correctly:\n\n%s", path, string(bs))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
