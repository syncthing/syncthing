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

package model

import (
	"os"
	"testing"
)

func TestSourceFileOK(t *testing.T) {
	s := sharedPullerState{
		realName: "testdata/foo",
	}

	fd, err := s.sourceFile()
	if err != nil {
		t.Fatal(err)
	}
	if fd == nil {
		t.Fatal("Unexpected nil fd")
	}

	bs := make([]byte, 6)
	n, err := fd.Read(bs)

	if n != len(bs) {
		t.Fatalf("Wrong read length %d != %d", n, len(bs))
	}
	if string(bs) != "foobar" {
		t.Fatalf("Wrong contents %s != foobar", string(bs))
	}

	if err := s.failed(); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFileBad(t *testing.T) {
	s := sharedPullerState{
		realName: "nonexistent",
	}

	fd, err := s.sourceFile()
	if err == nil {
		t.Fatal("Unexpected nil error")
	}
	if fd != nil {
		t.Fatal("Unexpected non-nil fd")
	}
	if err := s.failed(); err == nil {
		t.Fatal("Unexpected nil failed()")
	}
}

// Test creating temporary file inside read-only directory
func TestReadOnlyDir(t *testing.T) {
	// Create a read only directory, clean it up afterwards.
	os.Mkdir("testdata/read_only_dir", 0555)
	defer func() {
		os.Chmod("testdata/read_only_dir", 0755)
		os.RemoveAll("testdata/read_only_dir")
	}()

	s := sharedPullerState{
		tempName: "testdata/read_only_dir/.temp_name",
	}

	fd, err := s.tempFile()
	if err != nil {
		t.Fatal(err)
	}
	if fd == nil {
		t.Fatal("Unexpected nil fd")
	}

	s.fail("Test done", nil)
}
