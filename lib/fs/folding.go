// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type caseType int

const (
	asciiLower caseType = iota
	asciiMixed
	nonAscii
)

// UnicodeLowercaseNormalized returns the Unicode lower case variant of s,
// having also normalized it to normalization form C.
func UnicodeLowercaseNormalized(s string) string {
	switch checkCase(s) {
	case asciiLower:
		return s
	case asciiMixed:
		return strings.ToLower(s)
	default:
		return norm.NFC.String(strings.Map(toLower, s))
	}
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

func checkCase(s string) caseType {
	c := asciiLower
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b > unicode.MaxASCII {
			return nonAscii
		}
		if 'A' <= b && b <= 'Z' {
			c = asciiMixed
		}
	}
	return c
}
