// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
)

func TestLabelConversion(t *testing.T) {
	os.RemoveAll("testdata/oldformat.db")
	defer os.RemoveAll("testdata/oldformat.db")
	os.RemoveAll("testdata/newformat.db")
	defer os.RemoveAll("testdata/newformat.db")

	if err := unzip("testdata/oldformat.db.zip", "testdata"); err != nil {
		t.Fatal(err)
	}

	odb, err := leveldb.OpenFile("testdata/oldformat.db", nil)
	if err != nil {
		t.Fatal(err)
	}

	ldb, err := leveldb.OpenFile("testdata/newformat.db", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = convertKeyFormat(odb, ldb); err != nil {
		t.Fatal(err)
	}
	ldb.Close()
	odb.Close()

	inst, err := Open("testdata/newformat.db")
	if err != nil {
		t.Fatal(err)
	}

	fs := NewFileSet("default", inst)
	files, deleted, _ := fs.GlobalSize()
	if files+deleted != 953 {
		// Expected number of global entries determined by
		// ../../bin/stindex testdata/oldformat.db/ | grep global | grep -c default
		t.Errorf("Conversion error, global list differs (%d != 953)", files+deleted)
	}

	files, deleted, _ = fs.LocalSize()
	if files+deleted != 953 {
		t.Errorf("Conversion error, device list differs (%d != 953)", files+deleted)
	}

	f := NewBlockFinder(inst)
	// [block] F:"default" H:1c25dea9003cc16216e2a22900be1ec1cc5aaf270442904e2f9812c314e929d8 N:"f/f2/f25f1b3e6e029231b933531b2138796d" I:3
	h := []byte{0x1c, 0x25, 0xde, 0xa9, 0x00, 0x3c, 0xc1, 0x62, 0x16, 0xe2, 0xa2, 0x29, 0x00, 0xbe, 0x1e, 0xc1, 0xcc, 0x5a, 0xaf, 0x27, 0x04, 0x42, 0x90, 0x4e, 0x2f, 0x98, 0x12, 0xc3, 0x14, 0xe9, 0x29, 0xd8}
	found := 0
	f.Iterate([]string{"default"}, h, func(folder, file string, idx int32) bool {
		if folder == "default" && file == filepath.FromSlash("f/f2/f25f1b3e6e029231b933531b2138796d") && idx == 3 {
			found++
		}
		return true
	})
	if found != 1 {
		t.Errorf("Found %d blocks instead of expected 1", found)
	}

	inst.Close()
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}
