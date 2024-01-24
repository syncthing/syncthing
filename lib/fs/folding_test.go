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

var asciiCases = []struct {
	name       string
	result     caseType
	resultName string
}{
	{"img_202401241010.jpg", asciiLower, "lowercase ASCII"},
	{"IMG_202401241010.jpg", asciiMixed, "mixedcase ASCII"},
	{"收购要约_2024.xlsx", nonAscii, "unicode"},
}

func TestCheckCase(t *testing.T) {
	for _, ac := range asciiCases {
		res := checkCase(ac.name)
		if res != ac.result {
			t.Errorf("checkCase(%q) => %d, expected %d (%s)", ac.name, res, ac.result, ac.resultName)
		}
	if checkCase("MiXeD") != asciiMixed {
		t.Errorf("Expected asciiMixed")
	}
	if checkCase("文字化け") != nonAscii {
		t.Errorf("Expected nonAscii")
	}
}

func TestUnicodeLowercaseNormalized(t *testing.T) {
	for _, tc := range caseCases {
		res := UnicodeLowercaseNormalized(tc[0])
		if res != tc[1] {
			t.Errorf("UnicodeLowercaseNormalized(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}

func BenchmarkUnicodeLowercaseMaybeChange(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, s := range caseCases {
			UnicodeLowercaseNormalized(s[0])
		}
	}
}

func BenchmarkUnicodeLowercaseNoChange(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, s := range caseCases {
			UnicodeLowercaseNormalized(s[1])
		}
	}
}
