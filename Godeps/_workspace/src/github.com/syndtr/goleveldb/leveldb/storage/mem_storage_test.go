// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"bytes"
	"testing"
)

func TestMemStorage(t *testing.T) {
	m := NewMemStorage()

	l, err := m.Lock()
	if err != nil {
		t.Fatal("storage lock failed(1): ", err)
	}
	_, err = m.Lock()
	if err == nil {
		t.Fatal("expect error for second storage lock attempt")
	} else {
		t.Logf("storage lock got error: %s (expected)", err)
	}
	l.Release()
	_, err = m.Lock()
	if err != nil {
		t.Fatal("storage lock failed(2): ", err)
	}

	f := m.GetFile(1, TypeTable)
	if f.Num() != 1 && f.Type() != TypeTable {
		t.Fatal("invalid file number and type")
	}
	w, _ := f.Create()
	w.Write([]byte("abc"))
	w.Close()
	if ff, _ := m.GetFiles(TypeAll); len(ff) != 1 {
		t.Fatal("invalid GetFiles len")
	}
	buf := new(bytes.Buffer)
	r, err := f.Open()
	if err != nil {
		t.Fatal("Open: got error: ", err)
	}
	buf.ReadFrom(r)
	r.Close()
	if got := buf.String(); got != "abc" {
		t.Fatalf("Read: invalid value, want=abc got=%s", got)
	}
	if _, err := f.Open(); err != nil {
		t.Fatal("Open: got error: ", err)
	}
	if _, err := m.GetFile(1, TypeTable).Open(); err == nil {
		t.Fatal("expecting error")
	}
	f.Remove()
	if ff, _ := m.GetFiles(TypeAll); len(ff) != 0 {
		t.Fatal("invalid GetFiles len", len(ff))
	}
	if _, err := f.Open(); err == nil {
		t.Fatal("expecting error")
	}
}
