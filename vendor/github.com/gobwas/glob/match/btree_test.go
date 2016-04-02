package match

import (
	"testing"
)

func TestBTree(t *testing.T) {
	for id, test := range []struct {
		tree BTree
		str  string
		exp  bool
	}{
		{
			NewBTree(NewText("abc"), NewSuper(), NewSuper()),
			"abc",
			true,
		},
		{
			NewBTree(NewText("a"), NewSingle(nil), NewSingle(nil)),
			"aaa",
			true,
		},
		{
			NewBTree(NewText("b"), NewSingle(nil), nil),
			"bbb",
			false,
		},
		{
			NewBTree(
				NewText("c"),
				NewBTree(
					NewSingle(nil),
					NewSuper(),
					nil,
				),
				nil,
			),
			"abc",
			true,
		},
	} {
		act := test.tree.Match(test.str)
		if act != test.exp {
			t.Errorf("#%d match %q error: act: %t; exp: %t", id, test.str, act, test.exp)
			continue
		}
	}
}

type fakeMatcher struct {
	len  int
	name string
}

func (f *fakeMatcher) Match(string) bool {
	return true
}

var i = 3

func (f *fakeMatcher) Index(s string) (int, []int) {
	seg := make([]int, 0, i)
	for x := 0; x < i; x++ {
		seg = append(seg, x)
	}
	return 0, seg
}
func (f *fakeMatcher) Len() int {
	return f.len
}
func (f *fakeMatcher) String() string {
	return f.name
}

func BenchmarkMatchBTree(b *testing.B) {
	l := &fakeMatcher{4, "left_fake"}
	r := &fakeMatcher{4, "right_fake"}
	v := &fakeMatcher{2, "value_fake"}

	// must be <= len(l + r + v)
	fixture := "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij"

	bt := NewBTree(v, l, r)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bt.Match(fixture)
		}
	})
}
