// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
