// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package osutil

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Glob implements filepath.Glob, but works with Windows long path prefixes.
// Deals with https://github.com/golang/go/issues/10577
func Glob(pattern string) (matches []string, err error) {
	if !hasMeta(pattern) {
		if _, err = os.Lstat(pattern); err != nil {
			return nil, nil
		}
		return []string{pattern}, nil
	}

	dir, file := filepath.Split(pattern)
	switch dir {
	case "":
		dir = "."
	case string(filepath.Separator):
		// nothing
	default:
		dir = dir[0 : len(dir)-1] // chop off trailing separator
	}

	if !hasMeta(dir) {
		return glob(dir, file, nil)
	}

	var m []string
	m, err = Glob(dir)
	if err != nil {
		return
	}
	for _, d := range m {
		matches, err = glob(d, file, matches)
		if err != nil {
			return
		}
	}
	return
}

func hasMeta(path string) bool {
	// Strip off Windows long path prefix if it exists.
	if strings.HasPrefix(path, "\\\\?\\") {
		path = path[4:]
	}
	// TODO(niemeyer): Should other magic characters be added here?
	return strings.IndexAny(path, "*?[") >= 0
}

func glob(dir, pattern string, matches []string) (m []string, e error) {
	m = matches
	fi, err := os.Stat(dir)
	if err != nil {
		return
	}
	if !fi.IsDir() {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()

	names, _ := d.Readdirnames(-1)
	sort.Strings(names)

	for _, n := range names {
		matched, err := filepath.Match(pattern, n)
		if err != nil {
			return m, err
		}
		if matched {
			m = append(m, filepath.Join(dir, n))
		}
	}
	return
}
