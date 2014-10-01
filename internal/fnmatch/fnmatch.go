// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
	pattern = strings.Replace(pattern, ".", "\\.", -1)
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
