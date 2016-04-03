package glob

import (
	"testing"
)

func TestLexGood(t *testing.T) {
	for id, test := range []struct {
		pattern string
		items   []item
	}{
		{
			pattern: "hello",
			items: []item{
				item{item_text, "hello"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "hello?",
			items: []item{
				item{item_text, "hello"},
				item{item_single, "?"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "hellof*",
			items: []item{
				item{item_text, "hellof"},
				item{item_any, "*"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "hello**",
			items: []item{
				item{item_text, "hello"},
				item{item_super, "**"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "[日-語]",
			items: []item{
				item{item_range_open, "["},
				item{item_range_lo, "日"},
				item{item_range_between, "-"},
				item{item_range_hi, "語"},
				item{item_range_close, "]"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "[!日-語]",
			items: []item{
				item{item_range_open, "["},
				item{item_not, "!"},
				item{item_range_lo, "日"},
				item{item_range_between, "-"},
				item{item_range_hi, "語"},
				item{item_range_close, "]"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "[日本語]",
			items: []item{
				item{item_range_open, "["},
				item{item_text, "日本語"},
				item{item_range_close, "]"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "[!日本語]",
			items: []item{
				item{item_range_open, "["},
				item{item_not, "!"},
				item{item_text, "日本語"},
				item{item_range_close, "]"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "{a,b}",
			items: []item{
				item{item_terms_open, "{"},
				item{item_text, "a"},
				item{item_separator, ","},
				item{item_text, "b"},
				item{item_terms_close, "}"},
				item{item_eof, ""},
			},
		},
		{
			pattern: "{[!日-語],*,?,{a,b,\\c}}",
			items: []item{
				item{item_terms_open, "{"},
				item{item_range_open, "["},
				item{item_not, "!"},
				item{item_range_lo, "日"},
				item{item_range_between, "-"},
				item{item_range_hi, "語"},
				item{item_range_close, "]"},
				item{item_separator, ","},
				item{item_any, "*"},
				item{item_separator, ","},
				item{item_single, "?"},
				item{item_separator, ","},
				item{item_terms_open, "{"},
				item{item_text, "a"},
				item{item_separator, ","},
				item{item_text, "b"},
				item{item_separator, ","},
				item{item_text, "c"},
				item{item_terms_close, "}"},
				item{item_terms_close, "}"},
				item{item_eof, ""},
			},
		},
	} {
		lexer := newLexer(test.pattern)
		for i, exp := range test.items {
			act := lexer.nextItem()
			if act.t != exp.t {
				t.Errorf("#%d wrong %d-th item type: exp: %v; act: %v (%s vs %s)", id, i, exp.t, act.t, exp, act)
				break
			}
			if act.s != exp.s {
				t.Errorf("#%d wrong %d-th item contents: exp: %q; act: %q (%s vs %s)", id, i, exp.s, act.s, exp, act)
				break
			}
		}
	}
}
