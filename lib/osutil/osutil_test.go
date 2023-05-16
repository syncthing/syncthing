// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rand"
)

func TestIsDeleted(t *testing.T) {
	type tc struct {
		path  string
		isDel bool
	}
	cases := []tc{
		{"del", true},
		{"del.file", false},
		{filepath.Join("del", "del"), true},
		{"file", false},
		{"linkToFile", false},
		{"linkToDel", false},
		{"linkToDir", false},
		{filepath.Join("linkToDir", "file"), true},
		{filepath.Join("file", "behindFile"), true},
		{"dir", false},
		{"dir.file", false},
		{filepath.Join("dir", "file"), false},
		{filepath.Join("dir", "del"), true},
		{filepath.Join("dir", "del", "del"), true},
		{filepath.Join("del", "del", "del"), true},
	}

	testFs := fs.NewFilesystem(fs.FilesystemTypeFake, "testdata")

	testFs.MkdirAll("dir", 0o777)
	for _, f := range []string{"file", "del.file", "dir.file", filepath.Join("dir", "file")} {
		fd, err := testFs.Create(f)
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	for _, n := range []string{"Dir", "File", "Del"} {
		if err := testFs.CreateSymlink(strings.ToLower(n), "linkTo"+n); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range cases {
		if osutil.IsDeleted(testFs, c.path) != c.isDel {
			t.Errorf("IsDeleted(%v) != %v", c.path, c.isDel)
		}
	}
}

func TestRenameOrCopy(t *testing.T) {
	sameFs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32)+"?content=true")
	tests := []struct {
		src  fs.Filesystem
		dst  fs.Filesystem
		file string
	}{
		{
			src:  sameFs,
			dst:  sameFs,
			file: "file",
		},
		{
			src:  fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32)+"?content=true"),
			dst:  fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32)+"?content=true"),
			file: "file",
		},
		{
			src:  fs.NewFilesystem(fs.FilesystemTypeFake, `fake://fake/?files=1&seed=42`),
			dst:  fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32)+"?content=true"),
			file: osutil.NativeFilename(`05/7a/4d52f284145b9fe8`),
		},
	}

	for _, test := range tests {
		content := test.src.URI()
		if _, err := test.src.Lstat(test.file); err != nil {
			if !fs.IsNotExist(err) {
				t.Fatal(err)
			}
			if fd, err := test.src.Create(test.file); err != nil {
				t.Fatal(err)
			} else {
				if _, err := fd.Write([]byte(test.src.URI())); err != nil {
					t.Fatal(err)
				}
				_ = fd.Close()
			}
		} else {
			fd, err := test.src.Open(test.file)
			if err != nil {
				t.Fatal(err)
			}
			buf, err := io.ReadAll(fd)
			if err != nil {
				t.Fatal(err)
			}
			_ = fd.Close()
			content = string(buf)
		}

		err := osutil.RenameOrCopy(fs.CopyRangeMethodStandard, test.src, test.dst, test.file, "new")
		if err != nil {
			t.Fatal(err)
		}

		if fd, err := test.dst.Open("new"); err != nil {
			t.Fatal(err)
		} else {
			t.Cleanup(func() {
				_ = fd.Close()
			})

			if buf, err := io.ReadAll(fd); err != nil {
				t.Fatal(err)
			} else if string(buf) != content {
				t.Fatalf("expected %s got %s", content, string(buf))
			}
		}
	}
}
