// Copyright (C) 2014 The Protocol Authors.

// +build windows

package protocol

// Windows uses backslashes as file separator

import (
	"path/filepath"
	"strings"
)

type nativeModel struct {
	Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
	files = fixupFiles(files)
	m.Model.Index(deviceID, folder, files)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
	files = fixupFiles(files)
	m.Model.IndexUpdate(deviceID, folder, files)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, hash []byte, weakHash uint32, fromTemporary bool, buf []byte) error {
	if strings.Contains(name, `\`) {
		l.Warnf("Dropping request for %s, contains invalid path separator", name)
		return ErrNoSuchFile
	}

	name = filepath.FromSlash(name)
	return m.Model.Request(deviceID, folder, name, offset, hash, weakHash, fromTemporary, buf)
}

func fixupFiles(files []FileInfo) []FileInfo {
	var out []FileInfo
	for i := range files {
		if strings.Contains(files[i].Name, `\`) {
			l.Warnf("Dropping index entry for %s, contains invalid path separator", files[i].Name)
			if out == nil {
				// Most incoming updates won't contain anything invalid, so
				// we delay the allocation and copy to output slice until we
				// really need to do it, then copy all the so-far valid
				// files to it.
				out = make([]FileInfo, i, len(files)-1)
				copy(out, files)
			}
			continue
		}

		// Fixup the path separators
		files[i].Name = filepath.FromSlash(files[i].Name)

		if out != nil {
			out = append(out, files[i])
		}
	}

	if out != nil {
		// We did some filtering
		return out
	}

	// Unchanged
	return files
}
