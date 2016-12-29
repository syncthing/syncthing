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
	l.Unlock()
	_, err = m.Lock()
	if err != nil {
		t.Fatal("storage lock failed(2): ", err)
	}

	w, err := m.Create(FileDesc{TypeTable, 1})
	if err != nil {
		t.Fatal("Storage.Create: ", err)
	}
	w.Write([]byte("abc"))
	w.Close()
	if fds, _ := m.List(TypeAll); len(fds) != 1 {
		t.Fatal("invalid GetFiles len")
	}
	buf := new(bytes.Buffer)
	r, err := m.Open(FileDesc{TypeTable, 1})
	if err != nil {
		t.Fatal("Open: got error: ", err)
	}
	buf.ReadFrom(r)
	r.Close()
	if got := buf.String(); got != "abc" {
		t.Fatalf("Read: invalid value, want=abc got=%s", got)
	}
	if _, err := m.Open(FileDesc{TypeTable, 1}); err != nil {
		t.Fatal("Open: got error: ", err)
	}
	if _, err := m.Open(FileDesc{TypeTable, 1}); err == nil {
		t.Fatal("expecting error")
	}
	m.Remove(FileDesc{TypeTable, 1})
	if fds, _ := m.List(TypeAll); len(fds) != 0 {
		t.Fatal("invalid GetFiles len", len(fds))
	}
	if _, err := m.Open(FileDesc{TypeTable, 1}); err == nil {
		t.Fatal("expecting error")
	}
}
