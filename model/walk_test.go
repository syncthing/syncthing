package model

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

var testdata = []struct {
	name string
	size int
	hash string
}{
	{"bar", 10, "2f72cc11a6fcd0271ecef8c61056ee1eb1243be3805bf9a9df98f92f7636b05c"},
	{"foo", 7, "aec070645fe53ee3b3763059376134f058cc337247c978add178b6ccdfb0019f"},
}

var correctIgnores = map[string][]string{
	"": {".*", "quux"},
}

func TestWalk(t *testing.T) {
	m := NewModel("testdata")
	files, ignores := m.Walk(false)

	if l1, l2 := len(files), len(testdata); l1 != l2 {
		t.Fatalf("Incorrect number of walked files %d != %d", l1, l2)
	}

	for i := range testdata {
		if n1, n2 := testdata[i].name, files[i].Name; n1 != n2 {
			t.Errorf("Incorrect file name %q != %q for case #%d", n1, n2, i)
		}

		if h1, h2 := fmt.Sprintf("%x", files[i].Blocks[0].Hash), testdata[i].hash; h1 != h2 {
			t.Errorf("Incorrect hash %q != %q for case #%d", h1, h2, i)
		}

		t0 := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		if mt := files[i].Modified; mt < t0 || mt > t1 {
			t.Errorf("Unrealistic modtime %d for test %d", mt, i)
		}
	}

	if !reflect.DeepEqual(ignores, correctIgnores) {
		t.Errorf("Incorrect ignores\n  %v\n  %v", correctIgnores, ignores)
	}
}

func TestIgnore(t *testing.T) {
	var patterns = map[string][]string{
		"":        {"t2"},
		"foo":     {"bar", "z*"},
		"foo/baz": {"quux", ".*"},
	}
	var files = []File{
		{Name: "foo/bar"},
		{Name: "foo/quux"},
		{Name: "foo/zuux"},
		{Name: "foo/qzuux"},
		{Name: "foo/baz/t1"},
		{Name: "foo/baz/t2"},
		{Name: "foo/baz/bar"},
		{Name: "foo/baz/quuxa"},
		{Name: "foo/baz/aquux"},
		{Name: "foo/baz/.quux"},
		{Name: "foo/baz/zquux"},
		{Name: "foo/baz/quux"},
		{Name: "foo/bazz/quux"},
	}
	var remaining = []File{
		{Name: "foo/quux"},
		{Name: "foo/qzuux"},
		{Name: "foo/baz/t1"},
		{Name: "foo/baz/quuxa"},
		{Name: "foo/baz/aquux"},
		{Name: "foo/bazz/quux"},
	}

	var filtered = ignoreFilter(patterns, files)
	if !reflect.DeepEqual(filtered, remaining) {
		t.Errorf("Filtering mismatch\n  %v\n  %v", remaining, filtered)
	}
}
