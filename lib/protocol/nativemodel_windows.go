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

func makeNative(m rawModel) rawModel { return nativeModel{m} }

type nativeModel struct {
	rawModel
}

func (m nativeModel) Index(idx *Index) error {
	idx.Files = fixupFiles(idx.Files)
	return m.rawModel.Index(idx)
}

func (m nativeModel) IndexUpdate(idxUp *IndexUpdate) error {
	idxUp.Files = fixupFiles(idxUp.Files)
	return m.rawModel.IndexUpdate(idxUp)
}

func (m nativeModel) Request(req *Request) (RequestResponse, error) {
	if strings.Contains(req.Name, `\`) {
		l.Warnf("Dropping request for %s, contains invalid path separator", req.Name)
		return nil, ErrNoSuchFile
	}

	req.Name = filepath.FromSlash(req.Name)
	return m.rawModel.Request(req)
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
