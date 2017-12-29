// Copyright 2017 The ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ql

import (
	"fmt"
	"go/scanner"
	"go/token"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/cznic/golex/lex"
)

const (
	ccEOF = iota + 0x80
	ccLetter
	ccDigit
	ccOther
)

func runeClass(r rune) int {
	switch {
	case r == lex.RuneEOF:
		return ccEOF
	case r < 0x80:
		return int(r)
	case unicode.IsLetter(r):
		return ccLetter
	case unicode.IsDigit(r):
		return ccDigit
	default:
		return ccOther
	}
}

type lexer struct {
	*lex.Lexer
	agg    []bool
	col    int
	errs   scanner.ErrorList
	expr   expression
	file   *token.File
	inj    int
	line   int
	list   []stmt
	params int
	root   bool
	sc     int
}

func newLexer(src string) (*lexer, error) {
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	l := &lexer{
		file: file,
	}
	l0, err := lex.New(
		file,
		strings.NewReader(src),
		lex.ErrorFunc(func(pos token.Pos, msg string) {
			l.errPos(pos, msg)
		}),
		lex.RuneClass(runeClass),
		lex.BOMMode(lex.BOMIgnoreFirst),
	)
	if err != nil {
		return nil, err
	}

	l.Lexer = l0
	return l, nil
}

func (l *lexer) errPos(pos token.Pos, format string, arg ...interface{}) {
	l.errs.Add(l.file.Position(pos), fmt.Sprintf(format, arg...))
}

func (l *lexer) err(s string, arg ...interface{}) {
	l.errPos(l.Last.Pos(), s, arg...)
}

// Implements yyLexer.
func (l *lexer) Error(s string) {
	l.err(s)
}

func (l *lexer) int(lval *yySymType, im bool) int {
	val := l.TokenBytes(nil)
	if im {
		val = val[:len(val)-1]
	}
	n, err := strconv.ParseUint(string(val), 0, 64)
	if err != nil {
		l.err("integer literal: %v", err)
		return int(unicode.ReplacementChar)
	}

	if im {
		lval.item = idealComplex(complex(0, float64(n)))
		return imaginaryLit
	}

	switch {
	case n < math.MaxInt64:
		lval.item = idealInt(n)
	default:
		lval.item = idealUint(n)
	}
	return intLit
}

func (l *lexer) float(lval *yySymType, im bool) int {
	val := l.TokenBytes(nil)
	if im {
		val = val[:len(val)-1]
	}
	n, err := strconv.ParseFloat(string(val), 64)
	if err != nil {
		l.err("float literal: %v", err)
		return int(unicode.ReplacementChar)
	}

	if im {
		lval.item = idealComplex(complex(0, n))
		return imaginaryLit
	}

	lval.item = idealFloat(n)
	return floatLit
}

func (l *lexer) str(lval *yySymType, pref string) int {
	val := l.TokenBytes(nil)
	l.sc = 0
	s := pref + string(val)
	s, err := strconv.Unquote(s)
	if err != nil {
		l.err("string literal: %v", err)
		return int(unicode.ReplacementChar)
	}

	lval.item = s
	return stringLit
}

func (l *lexer) npos() (line, col int) {
	pos := l.file.Position(l.Last.Pos())
	return pos.Line, pos.Column
}
