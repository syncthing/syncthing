package match

import (
	"reflect"
	"testing"
)

func TestListIndex(t *testing.T) {
	for id, test := range []struct {
		list     []rune
		not      bool
		fixture  string
		index    int
		segments []int
	}{
		{
			[]rune("ab"),
			false,
			"abc",
			0,
			[]int{1},
		},
		{
			[]rune("ab"),
			true,
			"fffabfff",
			0,
			[]int{1},
		},
	} {
		p := NewList(test.list, test.not)
		index, segments := p.Index(test.fixture)
		if index != test.index {
			t.Errorf("#%d unexpected index: exp: %d, act: %d", id, test.index, index)
		}
		if !reflect.DeepEqual(segments, test.segments) {
			t.Errorf("#%d unexpected segments: exp: %v, act: %v", id, test.segments, segments)
		}
	}
}

func BenchmarkIndexList(b *testing.B) {
	m := NewList([]rune("def"), false)

	for i := 0; i < b.N; i++ {
		m.Index(bench_pattern)
	}
}

func BenchmarkIndexListParallel(b *testing.B) {
	m := NewList([]rune("def"), false)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Index(bench_pattern)
		}
	})
}
