package match

import (
	"reflect"
	"testing"
)

func TestTextIndex(t *testing.T) {
	for id, test := range []struct {
		text     string
		fixture  string
		index    int
		segments []int
	}{
		{
			"b",
			"abc",
			1,
			[]int{1},
		},
		{
			"f",
			"abcd",
			-1,
			nil,
		},
	} {
		m := NewText(test.text)
		index, segments := m.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}

func BenchmarkIndexText(b *testing.B) {
	m := NewText("foo")

	for i := 0; i < b.N; i++ {
		_, s := m.Index(bench_pattern)
		releaseSegments(s)
	}
}

func BenchmarkIndexTextParallel(b *testing.B) {
	m := NewText("foo")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, s := m.Index(bench_pattern)
			releaseSegments(s)
		}
	})
}
