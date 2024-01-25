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
	var b strings.Builder
	var pos int
	isASCII, isLower := true, true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			isASCII = false
			break
		}
		if 'A' <= c && c <= 'Z' {
			if isLower {
				b.Grow(len(s))
				isLower = false
			}
			if pos != i {
				b.WriteString(s[pos:i])
			}
			pos = i + 1
			c += 'a' - 'A'
			b.WriteByte(c)
		}
	}

	if isASCII {
		if isLower {
			return s
		}
		if pos != len(s) {
			b.WriteString(s[pos:])
		}
		return b.String()
	}

	for i, r := range s {
		mapped := toLower(r)
		if r == mapped && isLower {
			continue
		}
		if isLower {
			b.Reset()
			b.Grow(len(s) + utf8.UTFMax + 1)
			b.WriteString(s[:i])
			isLower = false
		}
		b.WriteRune(mapped)
	}

	if isLower {
		return norm.NFC.String(s)
	}

	return norm.NFC.String(b.String())
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
