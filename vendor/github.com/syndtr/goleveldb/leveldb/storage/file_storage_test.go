// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var cases = []struct {
	oldName []string
	name    string
	ftype   FileType
	num     int64
}{
	{nil, "000100.log", TypeJournal, 100},
	{nil, "000000.log", TypeJournal, 0},
	{[]string{"000000.sst"}, "000000.ldb", TypeTable, 0},
	{nil, "MANIFEST-000002", TypeManifest, 2},
	{nil, "MANIFEST-000007", TypeManifest, 7},
	{nil, "9223372036854775807.log", TypeJournal, 9223372036854775807},
	{nil, "000100.tmp", TypeTemp, 100},
}

var invalidCases = []string{
	"",
	"foo",
	"foo-dx-100.log",
	".log",
	"",
	"manifest",
	"CURREN",
	"CURRENTX",
	"MANIFES",
	"MANIFEST",
	"MANIFEST-",
	"XMANIFEST-3",
	"MANIFEST-3x",
	"LOC",
	"LOCKx",
	"LO",
	"LOGx",
	"18446744073709551616.log",
	"184467440737095516150.log",
	"100",
	"100.",
	"100.lop",
}

func TestFileStorage_CreateFileName(t *testing.T) {
	for _, c := range cases {
		if name := fsGenName(FileDesc{c.ftype, c.num}); name != c.name {
			t.Errorf("invalid filename got '%s', want '%s'", name, c.name)
		}
	}
}

func TestFileStorage_ParseFileName(t *testing.T) {
	for _, c := range cases {
		for _, name := range append([]string{c.name}, c.oldName...) {
			fd, ok := fsParseName(name)
			if !ok {
				t.Errorf("cannot parse filename '%s'", name)
				continue
			}
			if fd.Type != c.ftype {
				t.Errorf("filename '%s' invalid type got '%d', want '%d'", name, fd.Type, c.ftype)
			}
			if fd.Num != c.num {
				t.Errorf("filename '%s' invalid number got '%d', want '%d'", name, fd.Num, c.num)
			}
		}
	}
}

func TestFileStorage_InvalidFileName(t *testing.T) {
	for _, name := range invalidCases {
		if fsParseNamePtr(name, nil) {
			t.Errorf("filename '%s' should be invalid", name)
		}
	}
}

func TestFileStorage_Locking(t *testing.T) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("goleveldb-testrwlock-%d", os.Getuid()))
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		t.Fatal("RemoveAll: got error: ", err)
	}
	defer os.RemoveAll(path)

	p1, err := OpenFile(path, false)
	if err != nil {
		t.Fatal("OpenFile(1): got error: ", err)
	}

	p2, err := OpenFile(path, false)
	if err != nil {
		t.Logf("OpenFile(2): got error: %s (expected)", err)
	} else {
		p2.Close()
		p1.Close()
		t.Fatal("OpenFile(2): expect error")
	}

	p1.Close()

	p3, err := OpenFile(path, false)
	if err != nil {
		t.Fatal("OpenFile(3): got error: ", err)
	}
	defer p3.Close()

	l, err := p3.Lock()
	if err != nil {
		t.Fatal("storage lock failed(1): ", err)
	}
	_, err = p3.Lock()
	if err == nil {
		t.Fatal("expect error for second storage lock attempt")
	} else {
		t.Logf("storage lock got error: %s (expected)", err)
	}
	l.Unlock()
	_, err = p3.Lock()
	if err != nil {
		t.Fatal("storage lock failed(2): ", err)
	}
}

func TestFileStorage_ReadOnlyLocking(t *testing.T) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("goleveldb-testrolock-%d", os.Getuid()))
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		t.Fatal("RemoveAll: got error: ", err)
	}
	defer os.RemoveAll(path)

	p1, err := OpenFile(path, false)
	if err != nil {
		t.Fatal("OpenFile(1): got error: ", err)
	}

	_, err = OpenFile(path, true)
	if err != nil {
		t.Logf("OpenFile(2): got error: %s (expected)", err)
	} else {
		t.Fatal("OpenFile(2): expect error")
	}

	p1.Close()

	p3, err := OpenFile(path, true)
	if err != nil {
		t.Fatal("OpenFile(3): got error: ", err)
	}

	p4, err := OpenFile(path, true)
	if err != nil {
		t.Fatal("OpenFile(4): got error: ", err)
	}

	_, err = OpenFile(path, false)
	if err != nil {
		t.Logf("OpenFile(5): got error: %s (expected)", err)
	} else {
		t.Fatal("OpenFile(2): expect error")
	}

	p3.Close()
	p4.Close()
}
