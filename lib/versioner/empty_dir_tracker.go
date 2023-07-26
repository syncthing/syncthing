// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"path/filepath"
	"sort"

	"github.com/syncthing/syncthing/lib/fs"
)

type emptyDirTracker map[string]struct{}

func (t emptyDirTracker) addDir(path string) {
	if path == "." {
		return
	}
	t[path] = struct{}{}
}

// Remove all dirs from the path to the file
func (t emptyDirTracker) addFile(path string) {
	dir := filepath.Dir(path)
	for dir != "." {
		delete(t, dir)
		dir = filepath.Dir(dir)
	}
}

func (t emptyDirTracker) emptyDirs() []string {
	var empty []string

	for dir := range t {
		empty = append(empty, dir)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(empty)))
	return empty
}

func (t emptyDirTracker) deleteEmptyDirs(fs fs.Filesystem) {
	for _, path := range t.emptyDirs() {
		l.Debugln("Cleaner: deleting empty directory", path)
		err := fs.Remove(path)
		if err != nil {
			l.Warnln("Versioner: can't remove directory", path, err)
		}
	}
}
