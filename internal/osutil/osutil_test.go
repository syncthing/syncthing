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

package osutil_test

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/internal/osutil"
)

func TestInWriteableDir(t *testing.T) {
	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	os.Mkdir("testdata", 0700)
	os.Mkdir("testdata/rw", 0700)
	os.Mkdir("testdata/ro", 0500)

	create := func(name string) error {
		fd, err := os.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	// These should succeed

	err = osutil.InWritableDir(create, "testdata/file")
	if err != nil {
		t.Error("testdata/file:", err)
	}
	err = osutil.InWritableDir(create, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}
	err = osutil.InWritableDir(os.Remove, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}

	err = osutil.InWritableDir(create, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}
	err = osutil.InWritableDir(os.Remove, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}

	// These should not

	err = osutil.InWritableDir(create, "testdata/nonexistent/foo")
	if err == nil {
		t.Error("testdata/nonexistent/foo returned nil error")
	}
	err = osutil.InWritableDir(create, "testdata/file/foo")
	if err == nil {
		t.Error("testdata/file/foo returned nil error")
	}
}
