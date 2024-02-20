// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import "runtime"

func init() {
	falseOnWindows := runtime.GOOS != "windows"

	et := FilesystemEncoderTypePassthrough
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
		"\uf000": true,
		"\uf02f": true, // encoded /
		"\uf05c": true, // encoded \
		//`"`:      falseOnWindows,
		//"*":      falseOnWindows,
		//":":      falseOnWindows,
		//"<":      falseOnWindows,
		//">":      falseOnWindows,
		//"?":      falseOnWindows,
		//"|":      falseOnWindows,
		//"\x00":   falseOnWindows,
		//"\x01":   falseOnWindows,
		//"\uf001": true,
	}

	for _, name := range windowsDisallowedNames {
		validPathCases[et][name] = falseOnWindows
	}
	for _, r := range windowsDisallowedCharacters {
		validPathCases[et][string([]rune{r})] = falseOnWindows
	}
}
