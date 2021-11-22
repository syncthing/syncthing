// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package meta

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Checks for forbidden words in all .go files
func TestForbiddenWords(t *testing.T) {
	checkDirs := []string{"../cmd", "../lib", "../test", "../script"}
	forbiddenWords := []string{
		`"io/ioutil"`, // deprecated and should not be imported
	}

	for _, dir := range checkDirs {
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

			bs, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			for _, word := range forbiddenWords {
				if bytes.Contains(bs, []byte(word)) {
					t.Errorf("%s: forbidden word %q", path, word)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
