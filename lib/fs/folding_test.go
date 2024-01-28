// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"testing"
)

var caseCases = [][2]string{
	{"", ""},
	{"hej", "hej"},
	{"HeJ!@#", "hej!@#"},
	// Western Europe diacritical stuff is trivial.
	{"ÜBERRÄKSMÖRGÅS", "überräksmörgås"},
	// As are ligatures.
	{"Æglefinus", "æglefinus"},
	{"Ĳssel", "ĳssel"},
	// Cyrillic seems regular as well.
	{"Привет", "привет"},
	// Greek has multiple lower case characters for things depending on
	// context; we should always choose the same one.
	{"Ὀδυσσεύς", "ὀδυσσεύσ"},
	{"ὈΔΥΣΣΕΎΣ", "ὀδυσσεύσ"},
	// German ß doesn't really have an upper case variant, and we
	// shouldn't mess things up when lower casing it either. We don't
	// attempt to make ß equivalent to "ss".
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
	// Nor katakana, as far as I can tell.
	{"チャーハン", "チャーハン"},
	// Some special Unicode characters, however, are folded by OSes.
	{"\u212A", "k"},
	// Folding renormalizes to NFC
	{"A\xCC\x88", "\xC3\xA4"}, // ä
	{"a\xCC\x88", "\xC3\xA4"}, // ä
}

var benchmarkCases = [][2]string{
	{"img_202401241010.jpg", "ASCII lowercase"},
	{"IMG_202401241010.jpg", "ASCII mixedcase start"},
	{"img_202401241010.JPG", "ASCII mixedcase end"},
	{"wir_kinder_aus_bullerbü.epub", "Unicode lowercase"},
	{"Wir_Kinder_aus_Bullerbü.epub", "Unicode mixedcase start"},
	{"wir_kinder_aus_bullerbü.EPUB", "Unicode mixedcase end"},
	{"translated_ウェブの国際化.html", "Multibyte Unicode lowercase"},
	{"Translated_ウェブの国際化.html", "Multibyte Unicode mixedcase start"},
	{"translated_ウェブの国際化.HTML", "Multibyte Unicode mixedcase end"},
}

func TestUnicodeLowercaseNormalized(t *testing.T) {
	for _, tc := range caseCases {
		res := UnicodeLowercaseNormalized(tc[0])
		if res != tc[1] {
			t.Errorf("UnicodeLowercaseNormalized(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}

func BenchmarkUnicodeLowercase(b *testing.B) {
	for _, c := range benchmarkCases {
		b.Run(c[1], func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				UnicodeLowercaseNormalized(c[0])
			}
		})
	}
}
