// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

var execExts map[string]bool

func init() {
	// PATHEXT contains a list of executable file extensions, on Windows
	pathext := filepath.SplitList(os.Getenv("PATHEXT"))
	// We want the extensions in execExts to be lower case
	execExts = make(map[string]bool, len(pathext))
	for _, ext := range pathext {
		execExts[strings.ToLower(ext)] = true
	}
}

// isWindowsExecutable returns true if the given path has an extension that is
// in the list of executable extensions.
func isWindowsExecutable(path string) bool {
	return execExts[strings.ToLower(filepath.Ext(path))]
}

func (e basicFileInfo) Mode() FileMode {
	m := e.FileInfo.Mode()
	if m&os.ModeSymlink != 0 && e.Size() > 0 {
		// "Symlinks" with nonzero size are in fact "hard" links, such as
		// NTFS deduped files. Remove the symlink bit.
		m &^= os.ModeSymlink
	}
	// Set executable bits on files with executable extenions (.exe, .bat, etc).
	if isWindowsExecutable(e.Name()) {
		m |= 0111
	}
	// There is no user/group/others in Windows' read-only attribute, and
	// all "w" bits are set if the file is not read-only.  Do not send these
	// group/others-writable bits to other devices in order to avoid
	// unexpected world-writable files on other platforms.
	m &^= 0022
	return FileMode(m)
}

func (e basicFileInfo) Owner() int {
	return -1
}

func (e basicFileInfo) Group() int {
	return -1
}

func (basicFileInfo) InodeChangeTime() time.Time {
	return time.Time{}
}

// osFileInfo converts e to os.FileInfo that is suitable
// to be passed to os.SameFile.
func (e *basicFileInfo) osFileInfo() os.FileInfo {
	fi := e.FileInfo
	if fi, ok := fi.(*dirJunctFileInfo); ok {
		return fi.FileInfo
	}
	return fi
}
