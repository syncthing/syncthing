// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/syncthing/syncthing/internal/slogutil"
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
	slices.SortFunc(empty, func(a, b string) int {
		return strings.Compare(b, a)
	})
	return empty
}

func (t emptyDirTracker) deleteEmptyDirs(fs fs.Filesystem) {
	for _, path := range t.emptyDirs() {
		l.Debugln("Cleaner: deleting empty directory", path)
		err := fs.Remove(path)
		if err != nil {
			slog.Warn("Failed to remove versioned directory", slogutil.FilePath(path), slogutil.Error(err))
		}
	}
}
