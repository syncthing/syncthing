// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fnmatch

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	FNM_NOESCAPE = (1 << iota)
	FNM_PATHNAME
	FNM_CASEFOLD
)

func Convert(pattern string, flags int) (*regexp.Regexp, error) {
	any := "."

	switch runtime.GOOS {
	case "windows":
		flags |= FNM_NOESCAPE | FNM_CASEFOLD
		pattern = filepath.FromSlash(pattern)
		if flags&FNM_PATHNAME != 0 {
			any = "[^\\\\]"
		}
	case "darwin":
		flags |= FNM_CASEFOLD
		fallthrough
	default:
		if flags&FNM_PATHNAME != 0 {
			any = "[^/]"
		}
	}

	if flags&FNM_NOESCAPE != 0 {
		pattern = strings.Replace(pattern, "\\", "\\\\", -1)
	} else {
		pattern = strings.Replace(pattern, "\\*", "[:escapedstar:]", -1)
		pattern = strings.Replace(pattern, "\\?", "[:escapedques:]", -1)
		pattern = strings.Replace(pattern, "\\.", "[:escapeddot:]", -1)
	}

	// Characters that are special in regexps but not in glob, must be
	// escaped.
	for _, char := range []string{".", "+", "$", "^", "(", ")", "|"} {
		pattern = strings.Replace(pattern, char, "\\"+char, -1)
	}

	pattern = strings.Replace(pattern, "**", "[:doublestar:]", -1)
	pattern = strings.Replace(pattern, "*", any+"*", -1)
	pattern = strings.Replace(pattern, "[:doublestar:]", ".*", -1)
	pattern = strings.Replace(pattern, "?", any, -1)

	pattern = strings.Replace(pattern, "[:escapedstar:]", "\\*", -1)
	pattern = strings.Replace(pattern, "[:escapedques:]", "\\?", -1)
	pattern = strings.Replace(pattern, "[:escapeddot:]", "\\.", -1)

	pattern = "^" + pattern + "$"
	if flags&FNM_CASEFOLD != 0 {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// Matches the pattern against the string, with the given flags,
// and returns true if the match is successful.
func Match(pattern, s string, flags int) (bool, error) {
	exp, err := Convert(pattern, flags)
	if err != nil {
		return false, err
	}
	return exp.MatchString(s), nil
}
