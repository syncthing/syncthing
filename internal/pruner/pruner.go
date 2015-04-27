// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package pruner

import (
	"path/filepath"
	"strings"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/db"
)

type Pruner struct {
	dirPatterns  []string
	filePatterns []string
}

func New(patterns []string) *Pruner {
	p := &Pruner{}
	for _, pattern := range patterns {
		if strings.HasSuffix(pattern, "[^/]+") {
			p.filePatterns = append(p.filePatterns, filepath.Dir(pattern))
		} else {
			p.dirPatterns = append(p.dirPatterns, pattern)
		}
	}
	return p
}

func (p *Pruner) ShouldSkipFile(path string) bool {
	if p == nil {
		return false
	}
	for _, pattern := range p.dirPatterns {
		// A file which is within a given directory pattern
		if strings.HasPrefix(path, pattern) {
			return false
		}
	}

	dir := filepath.Dir(path)

	for _, pattern := range p.filePatterns {
		// A file which matches one of the file patterns
		if pattern == dir {
			return false
		}
	}

	return true
}

func (p *Pruner) ShouldSkipDirectory(path string) bool {
	if p == nil {
		return false
	}
	for _, pattern := range p.dirPatterns {
		// A directory which is within a given pattern
		if strings.HasPrefix(path, pattern) {
			return false
		}
		// A directory which is required to get down to the given pattern
		if strings.HasPrefix(pattern, path) {
			return false
		}
	}
	return true
}

func (p *Pruner) ShouldSkip(file protocol.FileInfo) bool {
	if p == nil {
		return false
	}
	if file.IsSymlink() || !file.IsDirectory() {
		return p.ShouldSkipFile(file.Name)
	}
	return p.ShouldSkipDirectory(file.Name)
}

func (p *Pruner) ShouldSkipTruncated(file db.FileInfoTruncated) bool {
	if p == nil {
		return false
	}
	if file.IsSymlink() || !file.IsDirectory() {
		return p.ShouldSkipFile(file.Name)
	}
	return p.ShouldSkipDirectory(file.Name)
}
