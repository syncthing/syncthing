// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build windows

package protocol

// Windows uses backslashes as file separator and disallows a bunch of
// characters in the filename

import (
	"path/filepath"
	"strings"
)

var disallowedCharacters = string([]rune{
	'<', '>', ':', '"', '|', '?', '*',
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31,
})

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(nodeID NodeID, repo string, files []FileInfo) {
	for i, f := range files {
		if strings.ContainsAny(f.Name, disallowedCharacters) {
			if f.IsDeleted() {
				// Don't complain if the file is marked as deleted, since it
				// can't possibly exist here anyway.
				continue
			}
			files[i].Flags |= FlagInvalid
			l.Warnf("File name %q contains invalid characters; marked as invalid.", f.Name)
		}
		files[i].Name = filepath.FromSlash(f.Name)
	}
	m.next.Index(nodeID, repo, files)
}

func (m nativeModel) IndexUpdate(nodeID NodeID, repo string, files []FileInfo) {
	for i, f := range files {
		if strings.ContainsAny(f.Name, disallowedCharacters) {
			if f.IsDeleted() {
				// Don't complain if the file is marked as deleted, since it
				// can't possibly exist here anyway.
				continue
			}
			files[i].Flags |= FlagInvalid
			l.Warnf("File name %q contains invalid characters; marked as invalid.", f.Name)
		}
		files[i].Name = filepath.FromSlash(files[i].Name)
	}
	m.next.IndexUpdate(nodeID, repo, files)
}

func (m nativeModel) Request(nodeID NodeID, repo string, name string, offset int64, size int) ([]byte, error) {
	name = filepath.FromSlash(name)
	return m.next.Request(nodeID, repo, name, offset, size)
}

func (m nativeModel) ClusterConfig(nodeID NodeID, config ClusterConfigMessage) {
	m.next.ClusterConfig(nodeID, config)
}

func (m nativeModel) Close(nodeID NodeID, err error) {
	m.next.Close(nodeID, err)
}
