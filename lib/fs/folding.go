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
	for _, b := range []byte(s) {
		if b > unicode.MaxASCII {
			return false, isLower
		}
		if 'A' <= b && b <= 'Z' {
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
	for i, c := range []byte(s) {
		if c < 'A' || 'Z' < c {
			continue
		}
		if pos < i {
			b.WriteString(s[pos:i])
		}
		pos = i + 1
		b.WriteByte(c + 'a' - 'A')
	}
	if pos != len(s) {
		b.WriteString(s[pos:])
	}
	return b.String()
}

func toLowerUnicode(s string) string {
	i := firstCaseChange(s)
	if i == -1 {
		return norm.NFC.String(s)
	}

	var rs strings.Builder
	// WriteRune always reserves utf8.UTFMax bytes for non-ASCII runes,
	// even if it doesn't need all that space. Overallocate now to prevent
	// it from ever triggering a reallocation.
	rs.Grow(utf8.UTFMax - 1 + len(s))
	rs.WriteString(s[:i])

	for _, r := range s[i:] {
		if r <= unicode.MaxLatin1 && r != 'µ' {
			rs.WriteRune(unicode.ToLower(r))
		} else {
			rs.WriteRune(unicode.To(unicode.LowerCase, unicode.To(unicode.UpperCase, r)))
		}
	}
	return norm.NFC.String(rs.String())
}

// Byte index of the first rune r s.t. lower(upper(r)) != r.
func firstCaseChange(s string) int {
	for i, r := range s {
		if r <= unicode.MaxASCII {
			if r < 'A' || r > 'Z' {
				continue
			}
			return i
		}
		if unicode.To(unicode.LowerCase, unicode.To(unicode.UpperCase, r)) != r {
			return i
		}
	}
	return -1
}
