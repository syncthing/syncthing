// Copyright (C) 2016 The Protocol Authors.

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
