package glob

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	char_any           = '*'
	char_separator     = ','
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

var eof rune = 0

type stateFn func(*lexer) stateFn

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
	return fmt.Sprintf("%v<%s>", i.t, i.s)
}

type lexer struct {
	input       string
	start       int
	pos         int
	width       int
	runes       int
	termScopes  []int
	termPhrases map[int]int
	state       stateFn
	items       chan item
}

func newLexer(source string) *lexer {
	l := &lexer{
		input:       source,
		state:       lexText,
		items:       make(chan item, len(source)),
		termPhrases: make(map[int]int),
	}
	return l
}

func (l *lexer) run() {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.items)
}

func (l *lexer) nextItem() item {
	for {
		select {
		case item := <-l.items:
			return item
		default:
			if l.state == nil {
				return item{t: item_eof}
			}

			l.state = l.state(l)
		}
	}

	panic("something went wrong")
}

func (l *lexer) read() (r rune) {
	if l.pos >= len(l.input) {
		return eof
	}

	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += l.width
	l.runes++

	return
}

func (l *lexer) unread() {
	l.pos -= l.width
	l.runes--
}

func (l *lexer) reset() {
	l.pos = l.start
	l.runes = 0
}

func (l *lexer) ignore() {
	l.start = l.pos
	l.runes = 0
}

func (l *lexer) lookahead() rune {
	r := l.read()
	if r != eof {
		l.unread()
	}
	return r
}

func (l *lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.read()) != -1 {
		return true
	}
	l.unread()
	return false
}

func (l *lexer) acceptAll(valid string) {
	for strings.IndexRune(valid, l.read()) != -1 {
	}
	l.unread()
}

func (l *lexer) emit(t itemType) {
	if l.pos == len(l.input) {
		l.items <- item{t, l.input[l.start:]}
	} else {
		l.items <- item{t, l.input[l.start:l.pos]}
	}

	l.start = l.pos
	l.runes = 0
	l.width = 0
}

func (l *lexer) emitMaybe(t itemType) {
	if l.pos > l.start {
		l.emit(t)
	}
}

func (l *lexer) errorf(format string, args ...interface{}) {
	l.items <- item{item_error, fmt.Sprintf(format, args...)}
}

func lexText(l *lexer) stateFn {
	for {
		c := l.read()
		if c == eof {
			break
		}

		switch c {
		case char_escape:
			l.unread()
			l.emitMaybe(item_text)

			l.read()
			l.ignore()

			if l.read() == eof {
				l.errorf("unclosed '%s' character", string(char_escape))
				return nil
			}

		case char_single:
			l.unread()
			l.emitMaybe(item_text)
			return lexSingle

		case char_any:
			var n stateFn
			if l.lookahead() == char_any {
				n = lexSuper
			} else {
				n = lexAny
			}

			l.unread()
			l.emitMaybe(item_text)
			return n

		case char_range_open:
			l.unread()
			l.emitMaybe(item_text)
			return lexRangeOpen

		case char_terms_open:
			l.unread()
			l.emitMaybe(item_text)
			return lexTermsOpen

		case char_terms_close:
			l.unread()
			l.emitMaybe(item_text)
			return lexTermsClose

		case char_separator:
			l.unread()
			l.emitMaybe(item_text)
			return lexSeparator
		}
	}

	if l.pos > l.start {
		l.emit(item_text)
	}

	if len(l.termScopes) != 0 {
		l.errorf("invalid pattern syntax: unclosed terms")
		return nil
	}

	l.emit(item_eof)

	return nil
}

func lexInsideRange(l *lexer) stateFn {
	for {
		c := l.read()
		if c == eof {
			l.errorf("unclosed range construction")
			return nil
		}

		switch c {
		case char_range_not:
			// only first char makes sense
			if l.pos-l.width == l.start {
				l.emit(item_not)
			}

		case char_range_between:
			if l.runes != 2 {
				l.errorf("unexpected length of lo char inside range")
				return nil
			}

			l.reset()
			return lexRangeHiLo

		case char_range_close:
			l.unread()
			l.emitMaybe(item_text)
			return lexRangeClose
		}
	}
}

func lexRangeHiLo(l *lexer) stateFn {
	start := l.start

	for {
		c := l.read()
		if c == eof {
			l.errorf("unexpected end of input")
			return nil
		}

		switch c {
		case char_range_between:
			if l.runes != 1 {
				l.errorf("unexpected length of range: single character expected before minus")
				return nil
			}

			l.emit(item_range_between)

		case char_range_close:
			l.unread()

			if l.runes != 1 {
				l.errorf("unexpected length of range: single character expected before close")
				return nil
			}

			l.emit(item_range_hi)
			return lexRangeClose

		default:
			if start != l.start {
				continue
			}

			if l.runes != 1 {
				l.errorf("unexpected length of range: single character expected at the begining")
				return nil
			}

			l.emit(item_range_lo)
		}
	}
}

func lexAny(l *lexer) stateFn {
	l.pos += 1
	l.emit(item_any)
	return lexText
}

func lexSuper(l *lexer) stateFn {
	l.pos += 2
	l.emit(item_super)
	return lexText
}

func lexSingle(l *lexer) stateFn {
	l.pos += 1
	l.emit(item_single)
	return lexText
}

func lexSeparator(l *lexer) stateFn {
	if len(l.termScopes) == 0 {
		l.errorf("syntax error: separator not inside terms list")
		return nil
	}

	posOpen := l.termScopes[len(l.termScopes)-1]

	if l.pos-posOpen == 1 {
		l.errorf("syntax error: empty term before separator")
		return nil
	}

	l.termPhrases[posOpen] += 1
	l.pos += 1
	l.emit(item_separator)
	return lexText
}

func lexTermsOpen(l *lexer) stateFn {
	l.termScopes = append(l.termScopes, l.pos)
	l.pos += 1
	l.emit(item_terms_open)

	return lexText
}

func lexTermsClose(l *lexer) stateFn {
	if len(l.termScopes) == 0 {
		l.errorf("unexpected closing of terms: there is no opened terms")
		return nil
	}

	lastOpen := len(l.termScopes) - 1
	posOpen := l.termScopes[lastOpen]

	// if it is empty term
	if posOpen == l.pos-1 {
		l.errorf("term could not be empty")
		return nil
	}

	if l.termPhrases[posOpen] == 0 {
		l.errorf("term must contain >1 phrases")
		return nil
	}

	// cleanup
	l.termScopes = l.termScopes[:lastOpen]
	delete(l.termPhrases, posOpen)

	l.pos += 1
	l.emit(item_terms_close)

	return lexText
}

func lexRangeOpen(l *lexer) stateFn {
	l.pos += 1
	l.emit(item_range_open)
	return lexInsideRange
}

func lexRangeClose(l *lexer) stateFn {
	l.pos += 1
	l.emit(item_range_close)
	return lexText
}
