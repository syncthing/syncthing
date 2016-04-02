package glob

import (
	"github.com/gobwas/glob/match"
	"reflect"
	"testing"
)

var separators = []rune{'.'}

func TestGlueMatchers(t *testing.T) {
	for id, test := range []struct {
		in  []match.Matcher
		exp match.Matcher
	}{
		{
			[]match.Matcher{
				match.NewSuper(),
				match.NewSingle(nil),
			},
			match.NewMin(1),
		},
		{
			[]match.Matcher{
				match.NewAny(separators),
				match.NewSingle(separators),
			},
			match.EveryOf{match.Matchers{
				match.NewMin(1),
				match.NewContains(string(separators), true),
			}},
		},
		{
			[]match.Matcher{
				match.NewSingle(nil),
				match.NewSingle(nil),
				match.NewSingle(nil),
			},
			match.EveryOf{match.Matchers{
				match.NewMin(3),
				match.NewMax(3),
			}},
		},
		{
			[]match.Matcher{
				match.NewList([]rune{'a'}, true),
				match.NewAny([]rune{'a'}),
			},
			match.EveryOf{match.Matchers{
				match.NewMin(1),
				match.NewContains("a", true),
			}},
		},
	} {
		act, err := compileMatchers(test.in)
		if err != nil {
			t.Errorf("#%d convert matchers error: %s", id, err)
			continue
		}

		if !reflect.DeepEqual(act, test.exp) {
			t.Errorf("#%d unexpected convert matchers result:\nact: %#v;\nexp: %#v", id, act, test.exp)
			continue
		}
	}
}

func TestCompileMatchers(t *testing.T) {
	for id, test := range []struct {
		in  []match.Matcher
		exp match.Matcher
	}{
		{
			[]match.Matcher{
				match.NewSuper(),
				match.NewSingle(separators),
				match.NewText("c"),
			},
			match.NewBTree(
				match.NewText("c"),
				match.NewBTree(
					match.NewSingle(separators),
					match.NewSuper(),
					nil,
				),
				nil,
			),
		},
		{
			[]match.Matcher{
				match.NewAny(nil),
				match.NewText("c"),
				match.NewAny(nil),
			},
			match.NewBTree(
				match.NewText("c"),
				match.NewAny(nil),
				match.NewAny(nil),
			),
		},
		{
			[]match.Matcher{
				match.NewRange('a', 'c', true),
				match.NewList([]rune{'z', 't', 'e'}, false),
				match.NewText("c"),
				match.NewSingle(nil),
			},
			match.NewRow(
				4,
				match.Matchers{
					match.NewRange('a', 'c', true),
					match.NewList([]rune{'z', 't', 'e'}, false),
					match.NewText("c"),
					match.NewSingle(nil),
				}...,
			),
		},
	} {
		act, err := compileMatchers(test.in)
		if err != nil {
			t.Errorf("#%d convert matchers error: %s", id, err)
			continue
		}

		if !reflect.DeepEqual(act, test.exp) {
			t.Errorf("#%d unexpected convert matchers result:\nact: %#v\nexp: %#v", id, act, test.exp)
			continue
		}
	}
}

func TestConvertMatchers(t *testing.T) {
	for id, test := range []struct {
		in, exp []match.Matcher
	}{
		{
			[]match.Matcher{
				match.NewRange('a', 'c', true),
				match.NewList([]rune{'z', 't', 'e'}, false),
				match.NewText("c"),
				match.NewSingle(nil),
				match.NewAny(nil),
			},
			[]match.Matcher{
				match.NewRow(
					4,
					[]match.Matcher{
						match.NewRange('a', 'c', true),
						match.NewList([]rune{'z', 't', 'e'}, false),
						match.NewText("c"),
						match.NewSingle(nil),
					}...,
				),
				match.NewAny(nil),
			},
		},
		{
			[]match.Matcher{
				match.NewRange('a', 'c', true),
				match.NewList([]rune{'z', 't', 'e'}, false),
				match.NewText("c"),
				match.NewSingle(nil),
				match.NewAny(nil),
				match.NewSingle(nil),
				match.NewSingle(nil),
				match.NewAny(nil),
			},
			[]match.Matcher{
				match.NewRow(
					3,
					match.Matchers{
						match.NewRange('a', 'c', true),
						match.NewList([]rune{'z', 't', 'e'}, false),
						match.NewText("c"),
					}...,
				),
				match.NewMin(3),
			},
		},
	} {
		act := minimizeMatchers(test.in)
		if !reflect.DeepEqual(act, test.exp) {
			t.Errorf("#%d unexpected convert matchers 2 result:\nact: %#v\nexp: %#v", id, act, test.exp)
			continue
		}
	}
}

func pattern(nodes ...node) *nodePattern {
	return &nodePattern{
		nodeImpl: nodeImpl{
			desc: nodes,
		},
	}
}
func anyOf(nodes ...node) *nodeAnyOf {
	return &nodeAnyOf{
		nodeImpl: nodeImpl{
			desc: nodes,
		},
	}
}
func TestCompiler(t *testing.T) {
	for id, test := range []struct {
		ast    *nodePattern
		result Glob
		sep    []rune
	}{
		{
			ast:    pattern(&nodeText{text: "abc"}),
			result: match.NewText("abc"),
		},
		{
			ast:    pattern(&nodeAny{}),
			sep:    separators,
			result: match.NewAny(separators),
		},
		{
			ast:    pattern(&nodeAny{}),
			result: match.NewSuper(),
		},
		{
			ast:    pattern(&nodeSuper{}),
			result: match.NewSuper(),
		},
		{
			ast:    pattern(&nodeSingle{}),
			sep:    separators,
			result: match.NewSingle(separators),
		},
		{
			ast: pattern(&nodeRange{
				lo:  'a',
				hi:  'z',
				not: true,
			}),
			result: match.NewRange('a', 'z', true),
		},
		{
			ast: pattern(&nodeList{
				chars: "abc",
				not:   true,
			}),
			result: match.NewList([]rune{'a', 'b', 'c'}, true),
		},
		{
			ast: pattern(&nodeAny{}, &nodeSingle{}, &nodeSingle{}, &nodeSingle{}),
			sep: separators,
			result: match.EveryOf{Matchers: match.Matchers{
				match.NewMin(3),
				match.NewContains(string(separators), true),
			}},
		},
		{
			ast:    pattern(&nodeAny{}, &nodeSingle{}, &nodeSingle{}, &nodeSingle{}),
			result: match.NewMin(3),
		},
		{
			ast: pattern(&nodeAny{}, &nodeText{text: "abc"}, &nodeSingle{}),
			sep: separators,
			result: match.NewBTree(
				match.NewRow(
					4,
					match.Matchers{
						match.NewText("abc"),
						match.NewSingle(separators),
					}...,
				),
				match.NewAny(separators),
				nil,
			),
		},
		{
			ast: pattern(&nodeSuper{}, &nodeSingle{}, &nodeText{text: "abc"}, &nodeSingle{}),
			sep: separators,
			result: match.NewBTree(
				match.NewRow(
					5,
					match.Matchers{
						match.NewSingle(separators),
						match.NewText("abc"),
						match.NewSingle(separators),
					}...,
				),
				match.NewSuper(),
				nil,
			),
		},
		{
			ast:    pattern(&nodeAny{}, &nodeText{text: "abc"}),
			result: match.NewSuffix("abc"),
		},
		{
			ast:    pattern(&nodeText{text: "abc"}, &nodeAny{}),
			result: match.NewPrefix("abc"),
		},
		{
			ast:    pattern(&nodeText{text: "abc"}, &nodeAny{}, &nodeText{text: "def"}),
			result: match.NewPrefixSuffix("abc", "def"),
		},
		{
			ast:    pattern(&nodeAny{}, &nodeAny{}, &nodeAny{}, &nodeText{text: "abc"}, &nodeAny{}, &nodeAny{}),
			result: match.NewContains("abc", false),
		},
		{
			ast: pattern(&nodeAny{}, &nodeAny{}, &nodeAny{}, &nodeText{text: "abc"}, &nodeAny{}, &nodeAny{}),
			sep: separators,
			result: match.NewBTree(
				match.NewText("abc"),
				match.NewAny(separators),
				match.NewAny(separators),
			),
		},
		{
			ast: pattern(&nodeSuper{}, &nodeSingle{}, &nodeText{text: "abc"}, &nodeSuper{}, &nodeSingle{}),
			result: match.NewBTree(
				match.NewText("abc"),
				match.NewMin(1),
				match.NewMin(1),
			),
		},
		{
			ast:    pattern(anyOf(&nodeText{text: "abc"})),
			result: match.NewText("abc"),
		},
		{
			ast:    pattern(anyOf(pattern(anyOf(pattern(&nodeText{text: "abc"}))))),
			result: match.NewText("abc"),
		},
		{
			ast: pattern(anyOf(
				pattern(
					&nodeText{text: "abc"},
					&nodeSingle{},
				),
				pattern(
					&nodeText{text: "abc"},
					&nodeList{chars: "def"},
				),
				pattern(
					&nodeText{text: "abc"},
				),
				pattern(
					&nodeText{text: "abc"},
				),
			)),
			result: match.NewBTree(
				match.NewText("abc"),
				nil,
				match.AnyOf{Matchers: match.Matchers{
					match.NewSingle(nil),
					match.NewList([]rune{'d', 'e', 'f'}, false),
					match.NewNothing(),
				}},
			),
		},
		{
			ast: pattern(
				&nodeRange{lo: 'a', hi: 'z'},
				&nodeRange{lo: 'a', hi: 'x', not: true},
				&nodeAny{},
			),
			result: match.NewBTree(
				match.NewRow(
					2,
					match.Matchers{
						match.NewRange('a', 'z', false),
						match.NewRange('a', 'x', true),
					}...,
				),
				nil,
				match.NewSuper(),
			),
		},
		{
			ast: pattern(anyOf(
				pattern(
					&nodeText{text: "abc"},
					&nodeList{chars: "abc"},
					&nodeText{text: "ghi"},
				),
				pattern(
					&nodeText{text: "abc"},
					&nodeList{chars: "def"},
					&nodeText{text: "ghi"},
				),
			)),
			result: match.NewRow(
				7,
				match.Matchers{
					match.NewText("abc"),
					match.AnyOf{Matchers: match.Matchers{
						match.NewList([]rune{'a', 'b', 'c'}, false),
						match.NewList([]rune{'d', 'e', 'f'}, false),
					}},
					match.NewText("ghi"),
				}...,
			),
		},
		//				{
		//			ast: pattern(
		//				anyOf(&nodeText{text: "a"}, &nodeText{text: "b"}),
		//				anyOf(&nodeText{text: "c"}, &nodeText{text: "d"}),
		//			),
		//			result: match.AnyOf{Matchers: match.Matchers{
		//				match.NewRow(Matchers: match.Matchers{match.Raw{"a"}, match.Raw{"c", 1}}),
		//				match.NewRow(Matchers: match.Matchers{match.Raw{"a"}, match.Raw{"d"}}),
		//				match.NewRow(Matchers: match.Matchers{match.Raw{"b"}, match.Raw{"c", 1}}),
		//				match.NewRow(Matchers: match.Matchers{match.Raw{"b"}, match.Raw{"d"}}),
		//			}},
		//		},
	} {
		m, err := compile(test.ast, test.sep)
		if err != nil {
			t.Errorf("compilation error: %s", err)
			continue
		}

		if !reflect.DeepEqual(m, test.result) {
			t.Errorf("#%d results are not equal:\nexp: %#v\nact: %#v", id, test.result, m)
			continue
		}
	}
}

const complexityString = "abcd"

//func BenchmarkComplexityAny(b *testing.B) {
//	m := match.NewAny(nil)
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityContains(b *testing.B) {
//	m := match.NewContains()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityList(b *testing.B) {
//	m := match.NewList()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityMax(b *testing.B) {
//	m := match.NewMax()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityMin(b *testing.B) {
//	m := match.NewMin()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityNothing(b *testing.B) {
//	m := match.NewNothing()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityPrefix(b *testing.B) {
//	m := match.NewPrefix()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityPrefixSuffix(b *testing.B) {
//	m := match.NewPrefixSuffix()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityRange(b *testing.B) {
//	m := match.NewRange()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityRow(b *testing.B) {
//	m := match.NewRow()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexitySingle(b *testing.B) {
//	m := match.NewSingle(nil)
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexitySuffix(b *testing.B) {
//	m := match.NewSuffix()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexitySuper(b *testing.B) {
//	m := match.NewSuper()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityText(b *testing.B) {
//	m := match.NewText()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityAnyOf(b *testing.B) {
//	m := match.NewAnyOf()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityBTree(b *testing.B) {
//	m := match.NewBTree(match.NewText("abc"), match.NewText("d"), match.NewText("e"))
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
//func BenchmarkComplexityEveryOf(b *testing.B) {
//	m := match.NewEveryOf()
//	for i := 0; i < b.N; i++ {
//		_ = m.Match(complexityString)
//		_, _ = m.Index(complexityString)
//	}
//}
