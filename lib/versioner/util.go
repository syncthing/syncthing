// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Inserts ~tag just before the extension of the filename.
func TagFilename(name, tag string) string {
	dir, file := filepath.Dir(name), filepath.Base(name)
	ext := filepath.Ext(file)
	withoutExt := file[:len(file)-len(ext)]
	return filepath.Join(dir, withoutExt+"~"+tag+ext)
}

var tagExp = regexp.MustCompile(`.*~([^~.]+)(?:\.[^.]+)?$`)

// Returns the tag from a filename, whether at the end or middle.
func ExtractTag(path string) string {
	match := tagExp.FindStringSubmatch(path)
	// match is []string{"whole match", "submatch"} when successful

	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func UntagFilename(path string) (string, string) {
	ext := filepath.Ext(path)
	versionTag := ExtractTag(path)

	// Files tagged with old style tags cannot be untagged.
	if versionTag == "" || strings.HasSuffix(ext, versionTag) {
		return "", ""
	}

	withoutExt := path[:len(path)-len(ext)-len(versionTag)-1]
	name := withoutExt + ext
	return name, versionTag
}
