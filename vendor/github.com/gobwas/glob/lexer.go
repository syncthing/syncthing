package glob

import (
	"bytes"
	"fmt"
	"github.com/gobwas/glob/runes"
	"unicode/utf8"
)

const (
	char_any           = '*'
	char_comma         = ','
	char_single        = '?'
	char_escape        = '\\'
	char_range_open    = '['
	char_range_close   = ']'
	char_terms_open    = '{'
	char_terms_close   = '}'
	char_range_not     = '!'
	char_range_between = '-'
)

var specials = []byte{
	char_any,
	char_single,
	char_escape,
	char_range_open,
	char_range_close,
	char_terms_open,
	char_terms_close,
}

func special(c byte) bool {
	return bytes.IndexByte(specials, c) != -1
}

type itemType int

const (
	item_eof itemType = iota
	item_error
	item_text
	item_char
	item_any
	item_super
	item_single
	item_not
	item_separator
	item_range_open
	item_range_close
	item_range_lo
	item_range_hi
	item_range_between
	item_terms_open
	item_terms_close
)

func (i itemType) String() string {
	switch i {
	case item_eof:
		return "eof"

	case item_error:
		return "error"

	case item_text:
		return "text"

	case item_char:
		return "char"

	case item_any:
		return "any"

	case item_super:
		return "super"

	case item_single:
		return "single"

	case item_not:
		return "not"

	case item_separator:
		return "separator"

	case item_range_open:
		return "range_open"

	case item_range_close:
		return "range_close"

	case item_range_lo:
		return "range_lo"

	case item_range_hi:
		return "range_hi"

	case item_range_between:
		return "range_between"

	case item_terms_open:
		return "terms_open"

	case item_terms_close:
		return "terms_close"

	default:
		return "undef"
	}
}

type item struct {
	t itemType
	s string
}

func (i item) String() string {
	return fmt.Sprintf("%v<%q>", i.t, i.s)
}

type stubLexer struct {
	Items []item
	pos   int
}

func (s *stubLexer) nextItem() (ret item) {
	if s.pos == len(s.Items) {
		return item{item_eof, ""}
	}
	ret = s.Items[s.pos]
	s.pos++
	return
}

type items []item

func (i *items) shift() (ret item) {
	ret = (*i)[0]
	copy(*i, (*i)[1:])
	*i = (*i)[:len(*i)-1]
	return
}

func (i *items) push(v item) {
	*i = append(*i, v)
}

func (i *items) empty() bool {
	return len(*i) == 0
}

var eof rune = 0

type lexer struct {
	data string
	pos  int
	err  error

	items      items
	termsLevel int

	lastRune     rune
	lastRuneSize int
	hasRune      bool
}

func newLexer(source string) *lexer {
	l := &lexer{
		data:  source,
		items: items(make([]item, 0, 4)),
	}
	return l
}

func (l *lexer) peek() (r rune, w int) {
	if l.pos == len(l.data) {
		return eof, 0
	}

	r, w = utf8.DecodeRuneInString(l.data[l.pos:])
	if r == utf8.RuneError {
		l.errorf("could not read rune")
		r = eof
		w = 0
	}

	return
}

func (l *lexer) read() rune {
	if l.hasRune {
		l.hasRune = false
		l.seek(l.lastRuneSize)
		return l.lastRune
	}

	r, s := l.peek()
	l.seek(s)

	l.lastRune = r
	l.lastRuneSize = s

	return r
}

func (l *lexer) seek(w int) {
	l.pos += w
}

func (l *lexer) unread() {
	if l.hasRune {
		l.errorf("could not unread rune")
		return
	}
	l.seek(-l.lastRuneSize)
	l.hasRune = true
}

func (l *lexer) errorf(f string, v ...interface{}) {
	l.err = fmt.Errorf(f, v...)
}

func (l *lexer) inTerms() bool {
	return l.termsLevel > 0
}

func (l *lexer) termsEnter() {
	l.termsLevel++
}

func (l *lexer) termsLeave() {
	l.termsLevel--
}

func (l *lexer) nextItem() item {
	if l.err != nil {
		return item{item_error, l.err.Error()}
	}
	if !l.items.empty() {
		return l.items.shift()
	}

	l.fetchItem()
	return l.nextItem()
}

var inTextBreakers = []rune{char_single, char_any, char_range_open, char_terms_open}
var inTermsBreakers = append(inTextBreakers, char_terms_close, char_comma)

func (l *lexer) fetchItem() {
	r := l.read()
	switch {
	case r == eof:
		l.items.push(item{item_eof, ""})

	case r == char_terms_open:
		l.termsEnter()
		l.items.push(item{item_terms_open, string(r)})

	case r == char_comma && l.inTerms():
		l.items.push(item{item_separator, string(r)})

	case r == char_terms_close && l.inTerms():
		l.items.push(item{item_terms_close, string(r)})
		l.termsLeave()

	case r == char_range_open:
		l.items.push(item{item_range_open, string(r)})
		l.fetchRange()

	case r == char_single:
		l.items.push(item{item_single, string(r)})

	case r == char_any:
		if l.read() == char_any {
			l.items.push(item{item_super, string(r) + string(r)})
		} else {
			l.unread()
			l.items.push(item{item_any, string(r)})
		}

	default:
		l.unread()

		var breakers []rune
		if l.inTerms() {
			breakers = inTermsBreakers
		} else {
			breakers = inTextBreakers
		}
		l.fetchText(breakers)
	}
}

func (l *lexer) fetchRange() {
	var wantHi bool
	var wantClose bool
	var seenNot bool
	for {
		r := l.read()
		if r == eof {
			l.errorf("unexpected end of input")
			return
		}

		if wantClose {
			if r != char_range_close {
				l.errorf("expected close range character")
			} else {
				l.items.push(item{item_range_close, string(r)})
			}
			return
		}

		if wantHi {
			l.items.push(item{item_range_hi, string(r)})
			wantClose = true
			continue
		}

		if !seenNot && r == char_range_not {
			l.items.push(item{item_not, string(r)})
			seenNot = true
			continue
		}

		if n, w := l.peek(); n == char_range_between {
			l.seek(w)
			l.items.push(item{item_range_lo, string(r)})
			l.items.push(item{item_range_between, string(n)})
			wantHi = true
			continue
		}

		l.unread() // unread first peek and fetch as text
		l.fetchText([]rune{char_range_close})
		wantClose = true
	}
}

func (l *lexer) fetchText(breakers []rune) {
	var data []rune
	var escaped bool

reading:
	for {
		r := l.read()
		if r == eof {
			break
		}

		if !escaped {
			if r == char_escape {
				escaped = true
				continue
			}

			if runes.IndexRune(breakers, r) != -1 {
				l.unread()
				break reading
			}
		}

		escaped = false
		data = append(data, r)
	}

	if len(data) > 0 {
		l.items.push(item{item_text, string(data)})
	}
}
