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
	"unsafe"

	"golang.org/x/sys/windows"
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
	// Set executable bits on files with executable extensions (.exe, .bat, etc).
	if isWindowsExecutable(e.Name()) {
		m |= 0o111
	}
	// There is no user/group/others in Windows' read-only attribute, and
	// all "w" bits are set if the file is not read-only.  Do not send these
	// group/others-writable bits to other devices in order to avoid
	// unexpected world-writable files on other platforms.
	m &^= 0o022
	return FileMode(m)
}

func (e basicFileInfo) Owner() int {
	return -1
}

func (e basicFileInfo) Group() int {
	return -1
}

func inodeChangeTime(_ os.FileInfo, name string) time.Time {
	pathp, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return time.Time{}
	}

	attrs := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS | windows.FILE_FLAG_OPEN_REPARSE_POINT)

	h, err := windows.CreateFile(pathp, 0, 0, nil, windows.OPEN_EXISTING, attrs, 0)
	if err != nil {
		return time.Time{}
	}
	defer windows.CloseHandle(h) //nolint:errcheck // quiet linter

	var bi FILE_BASIC_INFO
	err = windows.GetFileInformationByHandleEx(h, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&bi)), uint32(unsafe.Sizeof(bi)))
	if err == nil {
		// ChangedTime is 100-nanosecond intervals since January 1, 1601.
		nsec := bi.ChangedTime
		// Change starting time to the Unix epoch (00:00:00 UTC, January 1, 1970).
		nsec -= 116444736000000000
		// Convert into nanoseconds.
		nsec *= 100

		return time.Unix(0, nsec)
	}

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

// See https://github.com/golang/go/blob/dbaa2d3e/src/internal/syscall/windows/syscall_windows.go#L162
type FILE_BASIC_INFO struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangedTime    int64
	FileAttributes uint32
	// Pad out to 8-byte alignment.
	_ uint32
}
