package match

import (
	"reflect"
	"testing"
)

func TestAnyIndex(t *testing.T) {
	for id, test := range []struct {
		sep      []rune
		fixture  string
		index    int
		segments []int
	}{
		{
			[]rune{'.'},
			"abc",
			0,
			[]int{0, 1, 2, 3},
		},
		{
			[]rune{'.'},
			"abc.def",
			0,
			[]int{0, 1, 2, 3},
		},
	} {
		p := NewAny(test.sep)
		index, segments := p.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}

func BenchmarkIndexAny(b *testing.B) {
	m := NewAny(bench_separators)

	for i := 0; i < b.N; i++ {
		_, s := m.Index(bench_pattern)
		releaseSegments(s)
	}
}

func BenchmarkIndexAnyParallel(b *testing.B) {
	m := NewAny(bench_separators)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, s := m.Index(bench_pattern)
			releaseSegments(s)
		}
	})
}
