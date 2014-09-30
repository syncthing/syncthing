// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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
