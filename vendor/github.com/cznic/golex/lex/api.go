// Copyright (c) 2015 The golex Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lex

import (
	"bytes"
	"fmt"
	"go/token"
	"io"
	"os"
)

// BOM handling modes which can be set by the BOMMode Option. Default is BOMIgnoreFirst.
const (
	BOMError       = iota // BOM is an error anywhere.
	BOMIgnoreFirst        // Skip BOM if at beginning, report as error if anywhere else.
	BOMPassAll            // No special handling of BOM.
	BOMPassFirst          // No special handling of BOM if at beginning, report as error if anywhere else.
)

const (
	NonASCII = 0x80 // DefaultRuneClass returns NonASCII for non ASCII runes.
	RuneEOF  = -1   // Distinct from any valid Unicode rune value.
)

// DefaultRuneClass returns the character class of r. If r is an ASCII code
// then its class equals the ASCII code. Any other rune is of class NonASCII.
//
// DefaultRuneClass is the default implementation Lexer will use to convert
// runes (21 bit entities) to scanner classes (8 bit entities).
//
// Non ASCII aware lexical analyzers will typically use their own
// categorization function. To assign such custom function use the RuneClass
// option.
func DefaultRuneClass(r rune) int {
	if r >= 0 && r < 0x80 {
		return int(r)
	}

	return NonASCII
}

// Char represents a rune and its position.
type Char struct {
	Rune rune
	pos  int32
}

// NewChar returns a new Char value.
func NewChar(pos token.Pos, r rune) Char { return Char{pos: int32(pos), Rune: r} }

// IsValid reports whether c is not a zero Char.
func (c Char) IsValid() bool { return c.Pos().IsValid() }

// Pos returns the token.Pos associated with c.
func (c Char) Pos() token.Pos { return token.Pos(c.pos) }

// CharReader is a RuneReader providing additionally explicit position
// information by returning a Char instead of a rune as its first result.
type CharReader interface {
	ReadChar() (c Char, size int, err error)
}

// Lexer suports golex[0] generated lexical analyzers.
type Lexer struct {
	File      *token.File             // The *token.File passed to New.
	First     Char                    // First remembers the lookahead char when Rule0 was invoked.
	Last      Char                    // Last remembers the last Char returned by Next.
	Prev      Char                    // Prev remembers the Char previous to Last.
	bomMode   int                     // See the BOM* constants.
	bytesBuf  bytes.Buffer            // Used by TokenBytes.
	charSrc   CharReader              // Lexer alternative input.
	classf    func(rune) int          //
	errorf    func(token.Pos, string) //
	lookahead Char                    // Lookahead if non zero.
	mark      int                     // Longest match marker.
	off       int                     // Used for File.AddLine.
	src       io.RuneReader           // Lexer input.
	tokenBuf  []Char                  // Lexeme collector.
	ungetBuf  []Char                  // Unget buffer.
}

// New returns a new *Lexer. The result can be amended using opts.
//
// Non Unicode Input
//
// To consume sources in other encodings and still have exact position
// information, pass an io.RuneReader which returns the next input character
// reencoded as an Unicode rune but returns the size (number of bytes used to
// encode it) of the original character, not the size of its UTF-8
// representation after converted to an Unicode rune.  Size is the second
// returned value of io.RuneReader.ReadRune method[4].
//
// When src optionally implements CharReader its ReadChar method is used
// instead of io.ReadRune.
func New(file *token.File, src io.RuneReader, opts ...Option) (*Lexer, error) {
	r := &Lexer{
		File:    file,
		bomMode: BOMIgnoreFirst,
		classf:  DefaultRuneClass,
		src:     src,
	}
	if x, ok := src.(CharReader); ok {
		r.charSrc = x
	}
	r.errorf = r.defaultErrorf
	for _, o := range opts {
		if err := o(r); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Abort handles the situation when the scanner does not successfully recognize
// any token or when an attempt to find the longest match "overruns" from an
// accepting state only to never reach an accepting state again. In the first
// case the scanner was never in an accepting state since last call to Rule0
// and then (true, previousLookahead rune) is returned, effectively consuming a
// single Char token, avoiding scanner stall.  Otherwise there was at least one
// accepting scanner state marked using Mark. In this case Abort rollbacks the
// lexer state to the marked state and returns (false, 0). The scanner must
// then execute a prescribed goto statement. For example:
//
//	%yyc c
//	%yyn c = l.Next()
//	%yym l.Mark()
//
//	%{
//	package foo
//
//	import (...)
//
//	type lexer struct {
//		*lex.Lexer
//		...
//	}
//
//	func newLexer(...) *lexer {
//		return &lexer{
//			lex.NewLexer(...),
//			...
//		}
//	}
//
//	func (l *lexer) scan() int {
//	        c := l.Enter()
//	%}
//
//	... more lex defintions
//
//	%%
//
//	        c = l.Rule0()
//
//	... lex rules
//
//	%%
//
//		if c, ok := l.Abort(); ok {
//			return c
//		}
//
//		goto yyAction
//	}
func (l *Lexer) Abort() (int, bool) {
	if l.mark >= 0 {
		if len(l.tokenBuf) > l.mark {
			l.Unget(l.lookahead)
			for i := len(l.tokenBuf) - 1; i >= l.mark; i-- {
				l.Unget(l.tokenBuf[i])
			}
		}
		l.tokenBuf = l.tokenBuf[:l.mark]
		return 0, false
	}

	switch n := len(l.tokenBuf); n {
	case 0: // [] z
		c := l.lookahead
		l.Next()
		return int(c.Rune), true
	case 1: // [a] z
		return int(l.tokenBuf[0].Rune), true
	default: // [a, b, ...], z
		c := l.tokenBuf[0]   // a
		l.Unget(l.lookahead) // z
		for i := n - 1; i > 1; i-- {
			l.Unget(l.tokenBuf[i]) // ...
		}
		l.lookahead = l.tokenBuf[1] // b
		l.tokenBuf = l.tokenBuf[:1]
		return int(c.Rune), true
	}
}

func (l *Lexer) class() int { return l.classf(l.lookahead.Rune) }

func (l *Lexer) defaultErrorf(pos token.Pos, msg string) {
	l.Error(fmt.Sprintf("%v: %v", l.File.Position(pos), msg))
}

// Enter ensures the lexer has a valid lookahead Char and returns its class.
// Typical use in an .l file
//
//	func (l *lexer) scan() lex.Char {
//		c := l.Enter()
//		...
func (l *Lexer) Enter() int {
	if !l.lookahead.IsValid() {
		l.Next()
	}
	return l.class()
}

// Error Implements yyLexer[2] by printing the msg to stderr.
func (l *Lexer) Error(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
}

// Lookahead returns the current lookahead.
func (l *Lexer) Lookahead() Char {
	if !l.lookahead.IsValid() {
		l.Next()
	}
	return l.lookahead
}

// Mark records the current state of scanner as accepting. It implements the
// golex macro %yym. Typical usage in an .l file:
//
//	%yym l.Mark()
func (l *Lexer) Mark() { l.mark = len(l.tokenBuf) }

func (l *Lexer) next() int {
	const bom = '\ufeff'

	if c := l.lookahead; c.IsValid() {
		l.tokenBuf = append(l.tokenBuf, c)
	}
	if n := len(l.ungetBuf); n != 0 {
		l.lookahead = l.ungetBuf[n-1]
		l.ungetBuf = l.ungetBuf[:n-1]
		return l.class()
	}

	if l.src == nil {
		return RuneEOF
	}

	var r rune
	var sz int
	var err error
	var pos token.Pos
	var c Char
again:
	off0 := l.off
	switch cs := l.charSrc; {
	case cs != nil:
		c, sz, err = cs.ReadChar()
		r = c.Rune
		pos = c.Pos()
	default:
		r, sz, err = l.src.ReadRune()
		pos = l.File.Pos(l.off)
	}
	l.off += sz
	if err != nil {
		l.src = nil
		r = RuneEOF
		if err != io.EOF {
			l.errorf(pos, err.Error())
		}
	}

	if r == bom {
		switch l.bomMode {
		default:
			fallthrough
		case BOMIgnoreFirst:
			if off0 != 0 {
				l.errorf(pos, "unicode (UTF-8) BOM in middle of file")
			}
			goto again
		case BOMPassAll:
			// nop
		case BOMPassFirst:
			if off0 != 0 {
				l.errorf(pos, "unicode (UTF-8) BOM in middle of file")
				goto again
			}
		case BOMError:
			switch {
			case off0 == 0:
				l.errorf(pos, "unicode (UTF-8) BOM at beginnig of file")
			default:
				l.errorf(pos, "unicode (UTF-8) BOM in middle of file")
			}
			goto again
		}
	}

	l.lookahead = NewChar(pos, r)
	if r == '\n' {
		l.File.AddLine(l.off)
	}
	return l.class()
}

// Next advances the scanner for one rune and returns the respective character
// class of the new lookahead.  Typical usage in an .l file:
//
//	%yyn c = l.Next()
func (l *Lexer) Next() int {
	l.Prev = l.Last
	r := l.next()
	l.Last = l.lookahead
	return r
}

// Offset returns the current reading offset of the lexer's source.
func (l *Lexer) Offset() int { return l.off }

// Rule0 initializes the scanner state before the attempt to recognize a token
// starts. The token collecting buffer is cleared.  Rule0 records the current
// lookahead in l.First and returns its class.  Typical usage in an .l file:
//
//	... lex definitions
//
//	%%
//
//		c := l.Rule0()
//
//	first-pattern-regexp
func (l *Lexer) Rule0() int {
	if !l.lookahead.IsValid() {
		l.Next()
	}
	l.First = l.lookahead
	l.mark = -1
	if len(l.tokenBuf) > 1<<18 { //DONE constant tuned
		l.tokenBuf = nil
	} else {
		l.tokenBuf = l.tokenBuf[:0]
	}
	return l.class()
}

// Token returns the currently collected token chars. The result is R/O.
func (l *Lexer) Token() []Char { return l.tokenBuf }

// TokenBytes returns the UTF-8 encoding of Token. If builder is not nil then
// it's called instead to build the encoded token byte value into the buffer
// passed to it.
//
// The Result is R/O.
func (l *Lexer) TokenBytes(builder func(*bytes.Buffer)) []byte {
	if len(l.bytesBuf.Bytes()) < 1<<18 { //DONE constant tuned
		l.bytesBuf.Reset()
	} else {
		l.bytesBuf = bytes.Buffer{}
	}
	switch {
	case builder != nil:
		builder(&l.bytesBuf)
	default:
		for _, c := range l.Token() {
			l.bytesBuf.WriteRune(c.Rune)
		}
	}
	return l.bytesBuf.Bytes()
}

// Unget unreads all chars in c.
func (l *Lexer) Unget(c ...Char) {
	l.ungetBuf = append(l.ungetBuf, c...)
	l.lookahead = Char{} // Must invalidate lookahead.
}

// Option is a function which can be passed as an optional argument to New.
type Option func(*Lexer) error

// BOMMode option selects how the lexer handles BOMs. See the BOM* constants for details.
func BOMMode(mode int) Option {
	return func(l *Lexer) error {
		l.bomMode = mode
		return nil
	}
}

// ErrorFunc option sets a function called when an, for example I/O error,
// occurs.  The default is to call Error with the position and message already
// formated as a string.
func ErrorFunc(f func(token.Pos, string)) Option {
	return func(l *Lexer) error {
		l.errorf = f
		return nil
	}
}

// RuneClass option sets the function used to convert runes to character
// classes.
func RuneClass(f func(rune) int) Option {
	return func(l *Lexer) error {
		l.classf = f
		return nil
	}
}
