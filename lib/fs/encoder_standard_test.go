// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import "runtime"

func init() {
	falseOnWindows := runtime.GOOS != "windows"

	et := FilesystemEncoderTypeStandard

	validPathCases[et] = stringBoolTestCases{
		// input, file is valid ? (result == nil) ?
		"":       true,
		".":      falseOnWindows,
		"..":     falseOnWindows,
		"ok":     true,
		"/":      true, // disallowed but missing from windowsDisallowedCharacters
		`\`:      true, // disallowed but missing from windowsDisallowedCharacters
		"dot.":   falseOnWindows,
		"space ": falseOnWindows,
		//"\x00":   false,
		"\uf000": true,
		"\uf001": false,
		"\uf02f": true, // encoded /
		"\uf05c": true, // encoded \
		//`"`:      falseOnWindows,
		//"*":      falseOnWindows,
		//":":      falseOnWindows,
		//"<":      falseOnWindows,
		//">":      falseOnWindows,
		//"?":      falseOnWindows,
		//"|":      falseOnWindows,
		//"\x01":   falseOnWindows,
	}

	for _, name := range windowsDisallowedNames {
		validPathCases[et][name] = falseOnWindows
	}
	for _, r := range windowsDisallowedCharacters {
		validPathCases[et][string([]rune{r})] = falseOnWindows
	}

	encodeNameCases[et] = encodeTestCases{
		// input, expected
		"":     "",
		".":    ".",
		"..":   "..",
		"::":   "::",
		"[:":   "[:",
		":[":   ":[",
		"/":    "/",
		`\`:    `\`,
		"\x00": "\x00",
		"\x01": "\x01",
	}

	// @TODO

	encodePathCases[et] = encodeTestCases{
		// input: expected
		`\`:         `\`,
		`\\`:        `\\`,
		`\.`:        `\.`,
		`\..`:       `\..`,
		`.\`:        `.\`,
		`..\`:       `..\`,
		"a":         "a",
		`a\`:        `a\`,
		`\a`:        `\a`,
		`\a\`:       `\a\`,
		"c:":        "c:",
		"c:a":       "c:a",
		`c:a\`:      `c:a\`,
		"c:.":       "c:.",
		"c:..":      "c:..",
		`c:\`:       `c:\`,
		`c:\\`:      `c:\\`,
		`c:\.`:      `c:\.`,
		`c:\..`:     `c:\..`,
		`\\?\c:\`:   `\\?\c:\`,
		`\\?\c:\\`:  `\\?\c:\\`,
		`\\?\c:\.`:  `\\?\c:\.`,
		`\\?\c:\..`: `\\?\c:\..`,
		"[:":        "[:",
		"[:a":       "[:a",
		`[:a\`:      "[:a\\",
		`[:\`:       "[:\\",
		`[:\\`:      "[:\\\\",
		`[:\.`:      "[:\\.",
		`[:\..`:     "[:\\..",
		"\\\x00":    "\\\x00",
		"\\\x01":    "\\\x01",
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
		{[]string{"\uf001"}, 0},
		{[]string{"\uf02f"}, 1}, // encoded /
		{[]string{"\uf05c"}, 1}, // encoded \
	}

	// @TODO

	wouldEncodeCases[et] = stringBoolTestCases{
		"ok":     false,
		":":      false,
		"dot.":   false,
		"space ": false,
		"\x00":   false,
		"\x01":   false,
		"\uf000": false,
		"\uf001": false,
		"\uf02f": false, // encoded /
		"\uf05c": false, // encoded \
	}

	// @TODO

}
