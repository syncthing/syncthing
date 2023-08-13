// Copyright (C) 2014 The Protocol Authors.

//go:build windows
// +build windows

package protocol

// Windows uses backslashes as file separator

import (
	"fmt"
	"path/filepath"
	"strings"
)

func makeNative(m contextLessModel) contextLessModel { return nativeModel{m} }

type nativeModel struct {
	contextLessModel
}

func (m nativeModel) Index(folder string, files []FileInfo) error {
	files = fixupFiles(files)
	return m.contextLessModel.Index(folder, files)
}

func (m nativeModel) IndexUpdate(folder string, files []FileInfo) error {
	files = fixupFiles(files)
	return m.contextLessModel.IndexUpdate(folder, files)
}

func (m nativeModel) Request(folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	if strings.Contains(name, `\`) {
		l.Warnf("Dropping request for %s, contains invalid path separator", name)
		return nil, ErrNoSuchFile
	}

	name = filepath.FromSlash(name)
	return m.contextLessModel.Request(folder, name, blockNo, size, offset, hash, weakHash, fromTemporary)
}

func fixupFiles(files []FileInfo) []FileInfo {
	var out []FileInfo
	for i := range files {
		if strings.Contains(files[i].Name, `\`) {
			msg := fmt.Sprintf("Dropping index entry for %s, contains invalid path separator", files[i].Name)
			if files[i].Deleted {
				// Dropping a deleted item doesn't have any consequences.
				l.Debugln(msg)
			} else {
				l.Warnln(msg)
			}
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
