// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
)

// generateTree generates n files with random data in a temporary directory
// and returns the path to the directory.
func generateTree(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		// Generate a random string. The first character is the directory
		// name, the rest is the file name.
		rnd := strings.ToLower(rand.String(16))
		sub := rnd[:1]
		file := rnd[1:]
		size := 512<<10 + rand.Intn(1024)<<10 // between 512 KiB and 1.5 MiB

		// Create the file with random data.
		os.Mkdir(filepath.Join(dir, sub), 0o700)
		lr := io.LimitReader(rand.Reader, int64(size))
		fd, err := os.Create(filepath.Join(dir, sub, file))
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(fd, lr)
		if err != nil {
			t.Fatal(err)
		}
		if err := fd.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// compareTrees compares the contents of two directories recursively. It
// reports any differences as test failures. Returns the number of files
// that were checked.
func compareTrees(t *testing.T, a, b string) int {
	t.Helper()

	// These will not match, so we ignore them.
	ignore := []string{".", ".stfolder"}

	nfiles := 0
	if err := filepath.Walk(a, func(path string, aInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(a, path)
		if err != nil {
			return err
		}

		if slices.Contains(ignore, rel) {
			return nil
		}

		bPath := filepath.Join(b, rel)
		bInfo, err := os.Stat(bPath)
		if err != nil {
			return err
		}

		if aInfo.IsDir() != bInfo.IsDir() {
			t.Errorf("mismatched directory/file: %q", rel)
		}

		if aInfo.Mode() != bInfo.Mode() {
			t.Errorf("mismatched mode: %q", rel)
		}

		if aInfo.Mode().IsRegular() {
			if !aInfo.ModTime().Equal(bInfo.ModTime()) {
				t.Errorf("mismatched mod time: %q", rel)
			}

			if aInfo.Size() != bInfo.Size() {
				t.Errorf("mismatched size: %q", rel)
			}

			aHash, err := sha256file(path)
			if err != nil {
				return err
			}
			bHash, err := sha256file(bPath)
			if err != nil {
				return err
			}
			if aHash != bHash {
				t.Errorf("mismatched hash: %q", rel)
			}

			nfiles++
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return nfiles
}

func sha256file(fname string) (string, error) {
	f, err := os.Open(fname)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	hb := h.Sum(nil)
	return fmt.Sprintf("%x", hb), nil
}
