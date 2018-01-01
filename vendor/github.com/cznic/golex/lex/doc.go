// Copyright (c) 2015 The golex Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lex is a Unicode-friendly run time library for golex[0] generated
// lexical analyzers[1].
//
// Changelog
//
// 2015-04-08: Initial release.
//
// Character classes
//
// Golex internally handles only 8 bit "characters". Many Unicode-aware
// tokenizers do not actually need to recognize every Unicode rune, but only
// some particular partitions/subsets. Like, for example, a particular Unicode
// category, say upper case letters: Lu.
//
// The idea is to convert all runes in a particular set as a single 8 bit
// character allocated outside the ASCII range of codes. The token value, a
// string of runes and their exact positions is collected as usual (see the
// Token and TokenBytes method), but the tokenizer DFA is simpler (and thus
// smaller and perhaps also faster) when this technique is used. In the example
// program (see below), recognizing (and skipping) white space, integer
// literals, one keyword and Go identifiers requires only an 8 state DFA[5].
//
// To provide the conversion from runes to character classes, "install" your
// converting function using the RuneClass option.
//
// References
//
// -
//
//  [0]: http://godoc.org/github.com/cznic/golex
//  [1]: http://en.wikipedia.org/wiki/Lexical_analysis
//  [2]: http://golang.org/cmd/yacc/
//  [3]: https://github.com/cznic/golex/blob/master/lex/example.l
//  [4]: http://golang.org/pkg/io/#RuneReader
//  [5]: https://github.com/cznic/golex/blob/master/lex/dfa
package lex
