// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

import "testing"

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
		t.Fatal("Wrong read length %d != %d", n, len(bs))
	}
	if string(bs) != "foobar" {
		t.Fatal("Wrong contents %s != foobar", bs)
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
