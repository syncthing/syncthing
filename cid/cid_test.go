// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package cid

import (
	"testing"

	"github.com/calmh/syncthing/protocol"
)

func TestGet(t *testing.T) {
	m := NewMap()

	fooID := protocol.NewNodeID([]byte("foo"))
	barID := protocol.NewNodeID([]byte("bar"))

	if i := m.Get(fooID); i != 1 {
		t.Errorf("Unexpected id %d != 1", i)
	}
	if i := m.Get(barID); i != 2 {
		t.Errorf("Unexpected id %d != 2", i)
	}
	if i := m.Get(fooID); i != 1 {
		t.Errorf("Unexpected id %d != 1", i)
	}
	if i := m.Get(barID); i != 2 {
		t.Errorf("Unexpected id %d != 2", i)
	}

	if LocalID != 0 {
		t.Error("LocalID should be 0")
	}
	if i := m.Get(LocalNodeID); i != LocalID {
		t.Errorf("Unexpected id %d != %d", i, LocalID)
	}
}
