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
	NoEscape = (1 << iota)
	PathName
	CaseFold
)

func Convert(pattern string, flags int) (*regexp.Regexp, error) {
	any := "."

	switch runtime.GOOS {
	case "windows":
		flags |= NoEscape | CaseFold
		pattern = filepath.FromSlash(pattern)
		if flags&PathName != 0 {
			any = "[^\\\\]"
		}
	case "darwin":
		flags |= CaseFold
		fallthrough
	default:
		if flags&PathName != 0 {
			any = "[^/]"
		}
	}

	// Support case insensitive ignores. We do the loop because we may in some
	// circumstances end up with multiple insensitivity prefixes
	// ("(?i)(?i)foo"), which should be accepted.
	for ignore := strings.TrimPrefix(pattern, "(?i)"); ignore != pattern; ignore = strings.TrimPrefix(pattern, "(?i)") {
		flags |= CaseFold
		pattern = ignore
	}

	if flags&NoEscape != 0 {
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
	if flags&CaseFold != 0 {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// Match matches the pattern against the string, with the given flags, and
// returns true if the match is successful.
func Match(pattern, s string, flags int) (bool, error) {
	exp, err := Convert(pattern, flags)
	if err != nil {
		return false, err
	}
	return exp.MatchString(s), nil
}
