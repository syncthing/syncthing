// Copyright (C) 2014 The Syncthing Authors.
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

package versioner

import (
	"path/filepath"
	"regexp"
	"sort"
)

// Inserts ~tag just before the extension of the filename.
func taggedFilename(name, tag string) string {
	dir, file := filepath.Dir(name), filepath.Base(name)
	ext := filepath.Ext(file)
	withoutExt := file[:len(file)-len(ext)]
	return filepath.Join(dir, withoutExt+"~"+tag+ext)
}

var tagExp = regexp.MustCompile(`.*~([^~.]+)(?:\.[^.]+)?$`)

// Returns the tag from a filename, whether at the end or middle.
func filenameTag(path string) string {
	match := tagExp.FindStringSubmatch(path)
	// match is []string{"whole match", "submatch"} when successfull

	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func uniqueSortedStrings(strings []string) []string {
	seen := make(map[string]struct{}, len(strings))
	unique := make([]string, 0, len(strings))
	for _, str := range strings {
		_, ok := seen[str]
		if !ok {
			seen[str] = struct{}{}
			unique = append(unique, str)
		}
	}
	sort.Strings(unique)
	return unique
}
