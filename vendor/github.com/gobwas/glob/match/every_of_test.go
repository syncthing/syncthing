package match

import (
	"reflect"
	"testing"
)

func TestEveryOfIndex(t *testing.T) {
	for id, test := range []struct {
		matchers Matchers
		fixture  string
		index    int
		segments []int
	}{
		{
			Matchers{
				NewAny(nil),
				NewText("b"),
				NewText("c"),
			},
			"dbc",
			-1,
			nil,
		},
		{
			Matchers{
				NewAny(nil),
				NewPrefix("b"),
				NewSuffix("c"),
			},
			"abc",
			1,
			[]int{2},
		},
	} {
		everyOf := NewEveryOf(test.matchers...)
		index, segments := everyOf.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}
