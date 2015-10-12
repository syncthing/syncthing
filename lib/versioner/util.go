// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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
	// match is []string{"whole match", "submatch"} when successful

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
