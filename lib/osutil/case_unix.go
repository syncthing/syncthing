// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package osutil

import (
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/fs"
)

// RealCase returns the correct case for the given name.
// For case sensitive filesystem this means that it will return a different case
// only if the given name doesn't exist, but the "same" name with different case
// does.  E.g. if "Foo" is on disk, and name is "foo", it returns "Foo", but for
// the same name if both "Foo" and "foo" exist on disk, "foo" is returned. It may
// also return fs.ErrNotExist.
func RealCase(ffs fs.Filesystem, name string) (string, error) {
	out := "."
	inComps := strings.Split(name, string(fs.PathSeparator))
	inCompsLower := strings.Split(fs.UnicodeLowercase(name), string(fs.PathSeparator))
outer:
	for i := range inComps {
		names, err := ffs.DirNames(out)
		if err != nil {
			return "", err
		}
		candidate := ""
		for _, n := range names {
			if n == inComps[i] {
				out = filepath.Join(out, n)
				continue outer
			}
			if candidate == "" && fs.UnicodeLowercase(n) == inCompsLower[i] {
				candidate = n
			}
		}
		if candidate == "" {
			return "", fs.ErrNotExist
		}
		out = filepath.Join(out, candidate)
	}
	return out, nil
}
