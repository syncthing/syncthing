// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"reflect"
	"testing"
)

func TestFixupFiles(t *testing.T) {
	files := []FileInfo{
		{Name: "foo/bar"},
		{Name: `foo\bar`},
		{Name: "foo/baz"},
		{Name: "foo/quux"},
		{Name: `foo\fail`},
	}

	// Filenames should be slash converted, except files which already have
	// backslashes in them which are instead filtered out.
	expected := []FileInfo{
		{Name: `foo\bar`},
		{Name: `foo\baz`},
		{Name: `foo\quux`},
	}

	fixed := fixupFiles(files)
	if !reflect.DeepEqual(fixed, expected) {
		t.Errorf("Got %v, expected %v", fixed, expected)
	}
}
