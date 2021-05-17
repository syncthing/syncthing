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
	i := firstCaseChange(s)
	if i == -1 {
		return s
	}

	var rs strings.Builder
	// WriteRune always reserves utf8.UTFMax bytes for non-ASCII runes,
	// even if it doesn't need all that space. Overallocate now to prevent
	// it from ever triggering a reallocation.
	rs.Grow(utf8.UTFMax - 1 + len(s))
	rs.WriteString(s[:i])

	for _, r := range s[i:] {
		rs.WriteRune(unicode.ToLower(unicode.ToUpper(r)))
	}
	return norm.NFC.String(rs.String())
}

// Byte index of the first rune r s.t. lower(upper(r)) != r.
func firstCaseChange(s string) int {
	for i, r := range s {
		if r <= unicode.MaxASCII && (r < 'A' || r > 'Z') {
			continue
		}
		if unicode.ToLower(unicode.ToUpper(r)) != r {
			return i
		}
	}
	return -1
}
