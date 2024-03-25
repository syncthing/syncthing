// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"runtime"
)

func init() {
	falseOnWindows := runtime.GOOS != "windows"

	et := FilesystemEncoderTypeFat

	validPathCases[et] = stringBoolTestCases{
		// input, file is valid ? (result == nil) ?
		"":   true,
		".":  falseOnWindows,
		"..": falseOnWindows,
		"ok": true,
		//		`"`:      true,
		//		"*":      true,
		"/": true, // disallowed but missing from windowsDisallowedCharacters
		//		":":      true,
		//		"<":      true,
		//		">":      true,
		//		"?":      true,
		`\`: true, // disallowed but missing from windowsDisallowedCharacters
		//		"|":      true,
		"dot.":   falseOnWindows,
		"space ": falseOnWindows,
		//"\x00":   false,
		//		"\x01":   true,
		//"\uf000": true,
		//		"\uf001": false,
		"\uf02f": true, // encoded /
		"\uf05c": true, // encoded \
	}

	for _, name := range windowsDisallowedNames {
		validPathCases[et][name] = falseOnWindows
	}
	for _, r := range windowsDisallowedCharacters {
		validPathCases[et][string([]rune{r})] = falseOnWindows
	}
	for _, r := range fatCharsToEncode {
		validPathCases[et][string([]rune{r})] = true
		validPathCases[et][string([]rune{r | 0xf000})] = false
	}
	encodeNameCases[et] = encodeTestCases{
		// input, expected
		"":     "",
		".":    ".",
		"..":   "..",
		"::":   "\uf03a\uf03a",
		"[:":   "[\uf03a",
		":[":   "\uf03a[",
		"/":    "/",
		`\`:    `\`,
		"\x00": "\x00",
		"\x01": "\uf001",

		// trailing spaces and periods are handled when AllowReservedFilenames is set
		//{" ", "\uf020"},
		//{"...", "\uf02e\uf02e\uf02e"},
		//{"a ", "a\uf020"},
		//{"a.", "a\uf02e"},
		//{"a  ", "a\uf020\uf020"},
		//{"a. ", "a\uf02e\uf020"},
		//{"a .", "a\uf020\uf02e"},
		//{"o k", "o k"},
		//{"o.k", "o.k"},
		//{" ok", " ok"},
		//{".ok", ".ok"},
	}

	// @TODO

	encodePathCases[et] = encodeTestCases{
		// input: expected
		`\`:      `\`,
		`\\`:     `\\`,
		`\.`:     `\.`,
		`\..`:    `\..`,
		`.\`:     `.\`,
		`..\`:    `..\`,
		"a":      "a",
		`a\`:     `a\`,
		`\a`:     `\a`,
		`\a\`:    `\a\`,
		"[:":     "[\uf03a",
		"[:a":    "[\uf03aa",
		`[:a\`:   "[\uf03aa\\",
		`[:\`:    "[\uf03a\\",
		`[:\\`:   "[\uf03a\\\\",
		`[:\.`:   "[\uf03a\\.",
		`[:\..`:  "[\uf03a\\..",
		"\\\x00": "\\\x00",
		"\\\x01": "\\\uf001",

		// trailing spaces and periods are handled when AllowReservedFilenames is set
		//`\ \ `: "\\\uf020\\\uf020",
		//"c: ": "c:\uf020",
		//"c:...": "c:\uf02e\uf02e\uf02e",
		//`c:\ `: "c:\\\uf020",
		//`c:\...`: "c:\\\uf02e\uf02e\uf02e",
		//`c:\ \ `: "c:\\\uf020\\\uf020",
		//`\\?\c:\ `: "\\\\?\\c:\\\uf020",
		//`\\?\c:\...`: "\\\\?\\c:\\\uf02e\uf02e\uf02e",
		//`\\?\c:\ \ `: "\\\\?\\c:\\\uf020\\\uf020",
		//"[: ": "[\uf03a\uf020",
		//"[:.": "[\uf03a\uf02e",
		//"[:..": "[\uf03a\uf02e\uf02e",
		//"[:...": "[\uf03a\uf02e\uf02e\uf02e",
		//`[:\ `: "[\uf03a\\\uf020",
		//`[:\...`: "[\uf03a\\\uf02e\uf02e\uf02e",
		//`[:\ \ `: "[\uf03a\\\uf020\\\uf020",
	}

	// @TODO

	filterCases[et] = []filterTestCase{
		{[]string{"ok"}, 1},
		{[]string{":"}, 1},
		{[]string{"dot."}, 1},
		{[]string{"space "}, 1},
		{[]string{"\x00"}, 1},
		{[]string{"\x01"}, 1},
		{[]string{"\uf000"}, 1},
		{[]string{"\uf001"}, 1},
		{[]string{"\uf02f"}, 1}, // encoded /
		{[]string{"\uf05c"}, 1}, // encoded \
	}

	// @TODO

	wouldEncodeCases[et] = stringBoolTestCases{
		"ok":     false,
		":":      true,
		"dot.":   false,
		"space ": false,
		"\x00":   false,
		"\x01":   true,
		"\uf000": false,
		"\uf001": false,
		"\uf02f": false, // encoded /
		"\uf05c": false, // encoded \
	}

	// @TODO

}
