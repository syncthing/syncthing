package match

import (
	"reflect"
	"testing"
)

func TestPrefixIndex(t *testing.T) {
	for id, test := range []struct {
		prefix   string
		fixture  string
		index    int
		segments []int
	}{
		{
			"ab",
			"abc",
			0,
			[]int{2, 3},
		},
		{
			"ab",
			"fffabfff",
			3,
			[]int{2, 3, 4, 5},
		},
	} {
		p := NewPrefix(test.prefix)
		index, segments := p.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}

func BenchmarkIndexPrefix(b *testing.B) {
	m := NewPrefix("qew")

	for i := 0; i < b.N; i++ {
		_, s := m.Index(bench_pattern)
		releaseSegments(s)
	}
}

func BenchmarkIndexPrefixParallel(b *testing.B) {
	m := NewPrefix("qew")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, s := m.Index(bench_pattern)
			releaseSegments(s)
		}
	})
}
