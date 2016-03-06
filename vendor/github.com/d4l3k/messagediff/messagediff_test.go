package messagediff

import (
	"testing"
	"time"
)

type testStruct struct {
	A, b int
	C    []int
}

func TestPrettyDiff(t *testing.T) {
	testData := []struct {
		a, b  interface{}
		diff  string
		equal bool
	}{
		{
			true,
			false,
			"modified:  = false\n",
			false,
		},
		{
			true,
			0,
			"modified:  = 0\n",
			false,
		},
		{
			[]int{0, 1, 2},
			[]int{0, 1, 2, 3},
			"added: [3] = 3\n",
			false,
		},
		{
			[]int{0, 1, 2, 3},
			[]int{0, 1, 2},
			"removed: [3] = 3\n",
			false,
		},
		{
			[]int{0},
			[]int{1},
			"modified: [0] = 1\n",
			false,
		},
		{
			&[]int{0},
			&[]int{1},
			"modified: [0] = 1\n",
			false,
		},
		{
			map[string]int{"a": 1, "b": 2},
			map[string]int{"b": 4, "c": 3},
			"added: [\"c\"] = 3\nmodified: [\"b\"] = 4\nremoved: [\"a\"] = 1\n",
			false,
		},
		{
			testStruct{1, 2, []int{1}},
			testStruct{1, 3, []int{1, 2}},
			"added: .C[1] = 2\nmodified: .b = 3\n",
			false,
		},
		{
			nil,
			nil,
			"",
			true,
		},
		{
			&time.Time{},
			nil,
			"modified:  = <nil>\n",
			false,
		},
		{
			time.Time{},
			time.Time{},
			"",
			true,
		},
		{
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Time{},
			"modified: .loc = (*time.Location)(nil)\nmodified: .sec = 0\n",
			false,
		},
	}
	for i, td := range testData {
		diff, equal := PrettyDiff(td.a, td.b)
		if diff != td.diff {
			t.Errorf("%d. PrettyDiff(%#v, %#v) diff = %#v; not %#v", i, td.a, td.b, diff, td.diff)
		}
		if equal != td.equal {
			t.Errorf("%d. PrettyDiff(%#v, %#v) equal = %#v; not %#v", i, td.a, td.b, equal, td.equal)
		}
	}
}

func TestPathString(t *testing.T) {
	testData := []struct {
		in   Path
		want string
	}{{
		Path{StructField("test"), SliceIndex(1), MapKey{"blue"}, MapKey{12.3}},
		".test[1][\"blue\"][12.3]",
	}}
	for i, td := range testData {
		if out := td.in.String(); out != td.want {
			t.Errorf("%d. %#v.String() = %#v; not %#v", i, td.in, out, td.want)
		}
	}
}
