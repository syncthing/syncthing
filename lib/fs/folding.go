package fs

import (
	"unicode"
)

func UnicodeLowercase(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		rs[i] = unicode.ToLower(unicode.ToUpper(r))
	}
	return string(rs)
}
