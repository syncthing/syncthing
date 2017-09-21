// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"fmt"
	"strings"
)

// An alphabet is a string of N characters, representing the digits of a given
// base N.
type luhnAlphabet string

var (
	luhnBase32 luhnAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
)

// generate returns a check digit for the string s, which should be composed
// of characters from the Alphabet a.
func (a luhnAlphabet) generate(s string) (rune, error) {
	factor := 1
	sum := 0
	n := len(a)

	for i := range s {
		codepoint := strings.IndexByte(string(a), s[i])
		if codepoint == -1 {
			return 0, fmt.Errorf("Digit %q not valid in alphabet %q", s[i], a)
		}
		addend := factor * codepoint
		if factor == 2 {
			factor = 1
		} else {
			factor = 2
		}
		addend = (addend / n) + (addend % n)
		sum += addend
	}
	remainder := sum % n
	checkCodepoint := (n - remainder) % n
	return rune(a[checkCodepoint]), nil
}

// luhnValidate returns true if the last character of the string s is correct, for
// a string s composed of characters in the alphabet a.
func (a luhnAlphabet) luhnValidate(s string) bool {
	t := s[:len(s)-1]
	c, err := a.generate(t)
	if err != nil {
		return false
	}
	return rune(s[len(s)-1]) == c
}
