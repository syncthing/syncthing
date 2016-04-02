package match

import (
	"reflect"
	"testing"
)

func TestSingleIndex(t *testing.T) {
	for id, test := range []struct {
		separators []rune
		fixture    string
		index      int
		segments   []int
	}{
		{
			[]rune{'.'},
			".abc",
			1,
			[]int{1},
		},
		{
			[]rune{'.'},
			".",
			-1,
			nil,
		},
	} {
		p := NewSingle(test.separators)
		index, segments := p.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}

func BenchmarkIndexSingle(b *testing.B) {
	m := NewSingle(bench_separators)

	for i := 0; i < b.N; i++ {
		_, s := m.Index(bench_pattern)
		releaseSegments(s)
	}
}

func BenchmarkIndexSingleParallel(b *testing.B) {
	m := NewSingle(bench_separators)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, s := m.Index(bench_pattern)
			releaseSegments(s)
		}
	})
}
