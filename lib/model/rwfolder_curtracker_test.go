// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"reflect"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestCurrentTracker(t *testing.T) {
	tr := newCurrentTracker()

	tr.Started(protocol.FileInfo{Name: "foo"})
	tr.Started(protocol.FileInfo{Name: "bar"})
	tr.Started(protocol.FileInfo{Name: "baz"})

	have := tr.Current()
	want := []string{"foo", "bar", "baz"}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("Current() incorrect; have %v, want %v", have, want)
	}

	// Removing the last item and adding another should work
	tr.Completed(protocol.FileInfo{Name: "baz"}, nil)
	tr.Started(protocol.FileInfo{Name: "quux"})

	have = tr.Current()
	want = []string{"foo", "bar", "quux"}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("Current() incorrect; have %v, want %v", have, want)
	}

	// Removing the first item and adding another should work
	tr.Completed(protocol.FileInfo{Name: "foo"}, nil)
	tr.Started(protocol.FileInfo{Name: "quuux"})

	have = tr.Current()
	want = []string{"bar", "quux", "quuux"}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("Current() incorrect; have %v, want %v", have, want)
	}

	// Removing from the middle should work
	tr.Completed(protocol.FileInfo{Name: "quux"}, nil)

	have = tr.Current()
	want = []string{"bar", "quuux"}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("Current() incorrect; have %v, want %v", have, want)
	}

	// Remving all should work
	tr.Completed(protocol.FileInfo{Name: "bar"}, nil)
	tr.Completed(protocol.FileInfo{Name: "quuux"}, nil)

	have = tr.Current()
	want = []string{}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("Current() incorrect; have %v, want %v", have, want)
	}
}
