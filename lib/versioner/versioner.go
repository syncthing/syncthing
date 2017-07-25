// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package versioner implements common interfaces for file versioning and a
// simple default versioning scheme.
package versioner

import (
	"os"
	"path/filepath"
	"runtime"
)

type Versioner interface {
	Archive(filePath string) error
}

var Factories = map[string]func(folderID string, folderDir string, params map[string]string) Versioner{}

const (
	TimeFormat = "20060102-150405"
	TimeGlob   = "[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]" // glob pattern matching TimeFormat
)

func cleanSymlinks(dir string) {
	if runtime.GOOS == "windows" {
		// We don't do symlinks on Windows. Additionally, there may
		// be things that look like symlinks that are not, which we
		// should leave alone. Deduplicated files, for example.
		return
	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			l.Infoln("Removing incorrectly versioned symlink", path)
			os.Remove(path)
			return filepath.SkipDir
		}
		return nil
	})
}
