package match

import (
	"reflect"
	"testing"
)

func TestRowIndex(t *testing.T) {
	for id, test := range []struct {
		matchers Matchers
		length   int
		fixture  string
		index    int
		segments []int
	}{
		{
			Matchers{
				NewText("abc"),
				NewText("def"),
				NewSingle(nil),
			},
			7,
			"qweabcdefghij",
			3,
			[]int{7},
		},
		{
			Matchers{
				NewText("abc"),
				NewText("def"),
				NewSingle(nil),
			},
			7,
			"abcd",
			-1,
			nil,
		},
	} {
		p := NewRow(test.length, test.matchers...)
		index, segments := p.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}

func BenchmarkRowIndex(b *testing.B) {
	m := NewRow(
		7,
		Matchers{
			NewText("abc"),
			NewText("def"),
			NewSingle(nil),
		}...,
	)

	for i := 0; i < b.N; i++ {
		_, s := m.Index(bench_pattern)
		releaseSegments(s)
	}
}

func BenchmarkIndexRowParallel(b *testing.B) {
	m := NewRow(
		7,
		Matchers{
			NewText("abc"),
			NewText("def"),
			NewSingle(nil),
		}...,
	)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, s := m.Index(bench_pattern)
			releaseSegments(s)
		}
	})
}
