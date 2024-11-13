// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import "fmt"

var luhnBase32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

func codepoint32(b byte) int {
	switch {
	case 'A' <= b && b <= 'Z':
		return int(b - 'A')
	case '2' <= b && b <= '7':
		return int(b + 26 - '2')
	default:
		return -1
	}
}

// luhn32 returns a check digit for the string s, which should be composed
// of characters from the alphabet luhnBase32.
// Doesn't follow the actual Luhn algorithm
// see https://forum.syncthing.net/t/v0-9-0-new-node-id-format/478/6 for more.
func luhn32(s string) (rune, error) {
	factor := 1
	sum := 0
	const n = 32

	for i := range s {
		codepoint := codepoint32(s[i])
		if codepoint == -1 {
			return 0, fmt.Errorf("digit %q not valid in alphabet %q", s[i], luhnBase32)
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
	return rune(luhnBase32[checkCodepoint]), nil
}
