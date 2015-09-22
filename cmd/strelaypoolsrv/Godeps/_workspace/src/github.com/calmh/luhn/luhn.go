// Copyright (C) 2014 Jakob Borg

// Package luhn generates and validates Luhn mod N check digits.
package luhn

import (
	"fmt"
	"strings"
)

// An alphabet is a string of N characters, representing the digits of a given
// base N.
type Alphabet string

var (
	Base32 Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
)

// Generate returns a check digit for the string s, which should be composed
// of characters from the Alphabet a.
func (a Alphabet) Generate(s string) (rune, error) {
	if err := a.check(); err != nil {
		return 0, err
	}

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

// Validate returns true if the last character of the string s is correct, for
// a string s composed of characters in the alphabet a.
func (a Alphabet) Validate(s string) bool {
	t := s[:len(s)-1]
	c, err := a.Generate(t)
	if err != nil {
		return false
	}
	return rune(s[len(s)-1]) == c
}

// check returns an error if the given alphabet does not consist of unique characters
func (a Alphabet) check() error {
	cm := make(map[byte]bool, len(a))
	for i := range a {
		if cm[a[i]] {
			return fmt.Errorf("Digit %q non-unique in alphabet %q", a[i], a)
		}
		cm[a[i]] = true
	}
	return nil
}
