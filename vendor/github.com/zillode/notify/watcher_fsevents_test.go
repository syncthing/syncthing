// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,!kqueue

package notify

import (
	"reflect"
	"testing"
)

func TestSplitflags(t *testing.T) {
	cases := [...]struct {
		set   uint32
		flags []uint32
	}{
		{0, nil},
		{0xD, []uint32{0x1, 0x4, 0x8}},
		{0x0010 | 0x0040 | 0x0080 | 0x01000, []uint32{0x0010, 0x0040, 0x0080, 0x01000}},
		{0x40000 | 0x00100 | 0x00200, []uint32{0x00100, 0x00200, 0x40000}},
	}
	for i, cas := range cases {
		if flags := splitflags(cas.set); !reflect.DeepEqual(flags, cas.flags) {
			t.Errorf("want flags=%v; got %v (i=%d)", cas.flags, flags, i)
		}
	}
}

func TestWatchStrip(t *testing.T) {
	const (
		create = uint32(FSEventsCreated)
		remove = uint32(FSEventsRemoved)
		rename = uint32(FSEventsRenamed)
		write  = uint32(FSEventsModified)
		inode  = uint32(FSEventsInodeMetaMod)
		owner  = uint32(FSEventsChangeOwner)
	)
	cases := [...][]struct {
		path string
		flag uint32
		diff uint32
	}{
		// 1.
		{
			{"file", create | write, create | write},
			{"file", create | write | inode, write | inode},
		},
		// 2.
		{
			{"file", create, create},
			{"file", create | remove, remove},
			{"file", create | remove, create},
		},
		// 3.
		{
			{"file", create | write, create | write},
			{"file", create | write | owner, write | owner},
		},
		// 4.
		{
			{"file", create | write, create | write},
			{"file", write | inode, write | inode},
			{"file", remove | write | inode, remove},
		},
		{
			{"file", remove | write | inode, remove},
		},
	}
Test:
	for i, cas := range cases {
		if len(cas) == 0 {
			t.Log("skipped")
			continue
		}
		w := &watch{prev: make(map[string]uint32)}
		for j, cas := range cas {
			if diff := w.strip(cas.path, cas.flag); diff != cas.diff {
				t.Errorf("want diff=%v; got %v (i=%d, j=%d)", Event(cas.diff),
					Event(diff), i, j)
				continue Test
			}
		}
	}
}

// Test for cases 3) and 5) with shadowed write&create events.
//
// See comment for (flagdiff).diff method.
func TestWatcherShadowedWriteCreate(t *testing.T) {
	w := NewWatcherTest(t, "testdata/vfs.txt")
	defer w.Close()

	cases := [...]WCase{
		// i=0
		create(w, "src/github.com/rjeczalik/fs/.fs.go.swp"),
		// i=1
		write(w, "src/github.com/rjeczalik/fs/.fs.go.swp", []byte("XD")),
		// i=2
		write(w, "src/github.com/rjeczalik/fs/.fs.go.swp", []byte("XD")),
		// i=3
		remove(w, "src/github.com/rjeczalik/fs/.fs.go.swp"),
		// i=4
		create(w, "src/github.com/rjeczalik/fs/.fs.go.swp"),
		// i=5
		write(w, "src/github.com/rjeczalik/fs/.fs.go.swp", []byte("XD")),
	}

	w.ExpectAny(cases[:5]) // BUG(rjeczalik): #62
}
