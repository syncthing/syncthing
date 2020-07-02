// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"io/ioutil"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

func TestRenameOrCopy(t *testing.T) {
	mustTempDir := func() string {
		t.Helper()
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatal(err)
		}
		return tmpDir
	}
	sameFs := fs.NewFilesystem(fs.FilesystemTypeBasic, mustTempDir())
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
			src:  fs.NewFilesystem(fs.FilesystemTypeBasic, mustTempDir()),
			dst:  fs.NewFilesystem(fs.FilesystemTypeBasic, mustTempDir()),
			file: "file",
		},
		{
			src:  fs.NewFilesystem(fs.FilesystemTypeFake, `fake://fake/?files=1&seed=42`),
			dst:  fs.NewFilesystem(fs.FilesystemTypeBasic, mustTempDir()),
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
			buf, err := ioutil.ReadAll(fd)
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
			if buf, err := ioutil.ReadAll(fd); err != nil {
				t.Fatal(err)
			} else if string(buf) != content {
				t.Fatalf("expected %s got %s", content, string(buf))
			}
		}
	}
}
