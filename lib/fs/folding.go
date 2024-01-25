// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// UnicodeLowercaseNormalized returns the Unicode lower case variant of s,
// having also normalized it to normalization form C.
func UnicodeLowercaseNormalized(s string) string {
	if isASCII, isLower := isASCII(s); isASCII {
		if isLower {
			return s
		}
		return toLowerASCII(s)
	}

	return toLowerUnicode(s)
}

func isASCII(s string) (bool, bool) {
	isLower := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			return false, isLower
		}
		if 'A' <= c && c <= 'Z' {
			isLower = false
		}
	}
	return true, isLower
}

func toLowerASCII(s string) string {
	var (
		b   strings.Builder
		pos int
	)
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			if pos != i {
				b.WriteString(s[pos:i])
			}
			pos = i + 1
			c += 'a' - 'A'
			b.WriteByte(c)
		}
	}
	if pos != len(s) {
		b.WriteString(s[pos:])
	}
	return b.String()
}

func toLowerUnicode(s string) string {
	s = strings.Map(toLower, s)
	return norm.NFC.String(s)
}

func toLower(r rune) rune {
	if r <= unicode.MaxASCII {
		if r < 'A' || 'Z' < r {
			return r
		}
		return r + 'a' - 'A'
	}
	return unicode.ToLower(unicode.ToUpper(r))
}
