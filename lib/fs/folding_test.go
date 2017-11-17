// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import "testing"

func TestUnicodeLowercase(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"hej", "hej"},
		{"HeJ!@#", "hej!@#"},
		// Western Europe diacritical stuff is trivial
		{"ÜBERRÄKSMÖRGÅS", "überräksmörgås"},
		// Cyrillic seems regular as well
		{"Привет", "привет"},
		// Greek has multiple lower case characters for things depending on
		// context; we should always choose the right one.
		{"Ὀδυσσεύς", "ὀδυσσεύσ"},
		{"ὈΔΥΣΣΕΎΣ", "ὀδυσσεύσ"},
		// German ß doesn't really have an upper case variant, and we
		// shouldn't mess things up when lower casing it either. We don't
		// attempt to make ß equivalant to "ss".
		{"Reichwaldstraße", "reichwaldstraße"},
		// The Turks do their thing with the Is.... Like the Greek example
		// we pick just the one canonicalized "i" although you can argue
		// with this... From what I understand most operating systems don't
		// get this right anyway.
		{"İI", "ii"},
		// Arabic doesn't do case folding.
		{"العَرَبِيَّة", "العَرَبِيَّة"},
		// Neither does Hebrew.
		{"עברית", "עברית"},
		// Nor Chinese, in any variant.
		{"汉语/漢語 or 中文", "汉语/漢語 or 中文"},
		// Niether katakana as far as I can tell.
		{"チャーハン", "チャーハン"},
	}
	for _, tc := range cases {
		res := UnicodeLowercase(tc[0])
		if res != tc[1] {
			t.Errorf("UnicodeLowercase(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}
