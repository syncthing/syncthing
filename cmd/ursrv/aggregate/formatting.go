// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package aggregate

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	plusRe  = regexp.MustCompile(`(\+.*|\.dev\..*)$`)
	plusStr = "(+dev)"
)

func prettyCase(input string) string {
	output := ""
	for i, runeValue := range input {
		if i == 0 {
			runeValue = unicode.ToUpper(runeValue)
		} else if unicode.IsUpper(runeValue) {
			output += " "
		}
		output += string(runeValue)
	}
	return output
}

// transformVersion returns a version number formatted correctly, with all
// development versions aggregated into one.
func transformVersion(v string) string {
	if v == "unknown-dev" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	v = plusRe.ReplaceAllString(v, " "+plusStr)

	return v
}
