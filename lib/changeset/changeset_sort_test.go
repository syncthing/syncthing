// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"reflect"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestSortDeleteSubdir(t *testing.T) {
	// Subdirectories must be deleted before deleting parents

	cs := New(Options{RootPath: "testdata"})
	cs.Queue(protocol.FileInfo{
		Name:  "foo",
		Flags: protocol.FlagDirectory | protocol.FlagDeleted,
	})
	cs.Queue(protocol.FileInfo{
		Name:  "foo/bar",
		Flags: protocol.FlagDirectory | protocol.FlagDeleted,
	})
	cs.sortQueue()

	if cs.queue[0].Name != "foo/bar" || cs.queue[1].Name != "foo" {
		t.Errorf("Incorrect order in:\n%+v", cs.queue)
	}
}

func TestSortCreateDir(t *testing.T) {
	// Directories must be created before files inside them

	cs := New(Options{RootPath: "testdata"})
	cs.Queue(protocol.FileInfo{
		Name:   "foo/bar",
		Flags:  0777,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
	})
	cs.Queue(protocol.FileInfo{
		Name:  "foo",
		Flags: protocol.FlagDirectory,
	})
	cs.sortQueue()

	if cs.queue[0].Name != "foo" || cs.queue[1].Name != "foo/bar" {
		t.Errorf("Incorrect order in:\n%+v", cs.queue)
	}
}

func TestSortUpdateDeleteFile(t *testing.T) {
	// File deletes must be done before updates

	cs := New(Options{RootPath: "testdata"})
	cs.Queue(protocol.FileInfo{
		Name:   "foo",
		Flags:  0777,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
	})
	cs.Queue(protocol.FileInfo{
		Name:   "foo",
		Flags:  0777 | protocol.FlagDeleted,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[1].hash, Size: protocol.BlockSize}},
	})
	cs.sortQueue()

	if cs.queue[0].Flags&protocol.FlagDeleted == 0 || cs.queue[1].Flags&protocol.FlagDeleted != 0 {
		t.Errorf("Incorrect order in:\n%+v", cs.queue)
	}
}

func TestSortReuseFile(t *testing.T) {
	// File deletes must be done *after* updates to other files with the same
	// hash, so that we can reuse the file.

	cs := New(Options{RootPath: "testdata"})
	cs.Queue(protocol.FileInfo{
		Name:   "foo",
		Flags:  0777 | protocol.FlagDeleted,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
	})
	cs.Queue(protocol.FileInfo{
		Name:   "bar",
		Flags:  0777,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
	})
	cs.sortQueue()

	if cs.queue[0].Name != "bar" || cs.queue[1].Name != "foo" {
		t.Errorf("Incorrect order in:\n%+v", cs.queue)
	}
}

func TestSortNoReorder(t *testing.T) {
	// The queue should not be reordered when there are no constraints that
	// require it. We test this a couple of times, because a map based thing
	// may have random ordering.

	for i := 0; i < 100; i++ {
		cs := New(Options{RootPath: "testdata"})
		cs.Queue(protocol.FileInfo{
			Name:   "zfoo",
			Flags:  0777 | protocol.FlagDeleted,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
		})
		cs.Queue(protocol.FileInfo{
			Name:   "bar",
			Flags:  0777,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[1].hash, Size: protocol.BlockSize}},
		})
		cs.Queue(protocol.FileInfo{
			Name:   "xbaz",
			Flags:  0777 | protocol.FlagDeleted,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[2].hash, Size: protocol.BlockSize}},
		})
		cs.Queue(protocol.FileInfo{
			Name:   "quux",
			Flags:  0777,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[3].hash, Size: protocol.BlockSize}},
		})
		cs.sortQueue()

		if cs.queue[0].Name != "zfoo" || cs.queue[1].Name != "bar" || cs.queue[2].Name != "xbaz" || cs.queue[3].Name != "quux" {
			t.Fatalf("Incorrect order in:\n%+v", cs.queue)
		}
	}
}

func TestSortReorderPartly(t *testing.T) {
	// The queue should not be reordered when there are no constraints that
	// require it. However the pair in the middle must be reordered.

	for i := 0; i < 100; i++ {
		cs := New(Options{RootPath: "testdata"})
		cs.Queue(protocol.FileInfo{
			Name:   "zfoo",
			Flags:  0777 | protocol.FlagDeleted,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
		})
		cs.Queue(protocol.FileInfo{
			Name:  "xbaz/bar",
			Flags: 0777 | protocol.FlagDirectory,
		})
		cs.Queue(protocol.FileInfo{
			Name:   "xbaz",
			Flags:  0777,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[2].hash, Size: protocol.BlockSize}},
		})
		cs.Queue(protocol.FileInfo{
			Name:   "quux",
			Flags:  0777,
			Blocks: []protocol.BlockInfo{{Hash: testBlocks[3].hash, Size: protocol.BlockSize}},
		})
		cs.sortQueue()

		if cs.queue[0].Name != "zfoo" || cs.queue[1].Name != "xbaz" || cs.queue[2].Name != "xbaz/bar" || cs.queue[3].Name != "quux" {
			t.Fatalf("Incorrect order in:\n%+v", cs.queue)
		}
	}
}

func TestSortBringToFront(t *testing.T) {
	// Moving an element to front should work properly.

	cs := New(Options{RootPath: "testdata"})
	cs.Queue(protocol.FileInfo{
		Name:   "zfoo",
		Flags:  0777 | protocol.FlagDeleted,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[0].hash, Size: protocol.BlockSize}},
	})
	cs.Queue(protocol.FileInfo{
		Name:  "xbaz/bar",
		Flags: 0777 | protocol.FlagDirectory,
	})
	cs.Queue(protocol.FileInfo{
		Name:   "xbaz",
		Flags:  0777,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[2].hash, Size: protocol.BlockSize}},
	})
	cs.Queue(protocol.FileInfo{
		Name:   "quux",
		Flags:  0777,
		Blocks: []protocol.BlockInfo{{Hash: testBlocks[3].hash, Size: protocol.BlockSize}},
	})
	cs.sortQueue()

	// Order is now:
	// zfoo
	// xbaz
	// xbaz/bar
	// quux

	want := []string{"zfoo", "xbaz", "xbaz/bar", "quux"}
	have := cs.QueueNames()
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("Unexpected order, have %v, want %v", have, want)
	}

	// Moving something to front overrides dependency order

	cs.BringToFront("xbaz/bar")

	want = []string{"xbaz/bar", "zfoo", "xbaz", "quux"}
	have = cs.QueueNames()
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("Unexpected order, have %v, want %v", have, want)
	}

	// Doing it again is a no-op

	cs.BringToFront("xbaz/bar")

	want = []string{"xbaz/bar", "zfoo", "xbaz", "quux"}
	have = cs.QueueNames()
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("Unexpected order, have %v, want %v", have, want)
	}

	// Moving the last element to the front works too

	cs.BringToFront("quux")

	want = []string{"quux", "xbaz/bar", "zfoo", "xbaz"}
	have = cs.QueueNames()
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("Unexpected order, have %v, want %v", have, want)
	}
}
