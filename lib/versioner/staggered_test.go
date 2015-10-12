// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package versioner

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStaggeredVersioningVersionCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Test takes some time, skipping.")
	}

	dir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	if err != nil {
		t.Error(err)
	}

	v := NewStaggered("", dir, map[string]string{"maxAge": "365"})
	versionDir := filepath.Join(dir, ".stversions")

	path := filepath.Join(dir, "test")

	for i := 1; i <= 3; i++ {
		f, err := os.Create(path)
		if err != nil {
			t.Error(err)
		}
		f.Close()
		v.Archive(path)

		d, err := os.Open(versionDir)
		if err != nil {
			t.Error(err)
		}
		n, err := d.Readdirnames(-1)
		if err != nil {
			t.Error(err)
		}

		if len(n) != 1 {
			t.Error("Wrong count")
		}
		d.Close()

		time.Sleep(time.Second)
	}
	os.RemoveAll(path)

	for i := 1; i <= 3; i++ {
		f, err := os.Create(path)
		if err != nil {
			t.Error(err)
		}
		f.Close()
		v.Archive(path)

		d, err := os.Open(versionDir)
		if err != nil {
			t.Error(err)
		}
		n, err := d.Readdirnames(-1)
		if err != nil {
			t.Error(err)
		}

		if len(n) != i {
			t.Error("Wrong count")
		}
		d.Close()

		time.Sleep(31 * time.Second)
	}
	os.RemoveAll(path)
}
