// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (BasicFilesystem) SymlinksSupported() bool {
	return true
}

func (f *BasicFilesystem) CreateSymlink(target, name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Symlink(target, name)
}

func (f *BasicFilesystem) ReadSymlink(name string) (string, error) {
	name, err := f.rooted(name)
	if err != nil {
		return "", err
	}
	return os.Readlink(name)
}

func (f *BasicFilesystem) mkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Unhide is a noop on unix, as unhiding files requires renaming them.
// We still check that the relative path does not try to escape the root
func (f *BasicFilesystem) Unhide(name string) error {
	_, err := f.rooted(name)
	return err
}

// Hide is a noop on unix, as hiding files requires renaming them.
// We still check that the relative path does not try to escape the root
func (f *BasicFilesystem) Hide(name string) error {
	_, err := f.rooted(name)
	return err
}

func (f *BasicFilesystem) Roots() ([]string, error) {
	return []string{"/"}, nil
}

// unrootedChecked returns the path relative to the folder root (same as
// unrooted). It panics if the given path is not a subpath and handles the
// special case when the given path is the folder root without a trailing
// pathseparator.
func (f *BasicFilesystem) unrootedChecked(absPath, root string) string {
	if absPath+string(PathSeparator) == root {
		return "."
	}
	if !strings.HasPrefix(absPath, root) {
		panic(fmt.Sprintf("bug: Notify backend is processing a change outside of the filesystem root: f.root==%v, root==%v, path==%v", f.root, root, absPath))
	}
	return rel(absPath, root)
}

func rel(path, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, prefix), string(PathSeparator))
}

var evalSymlinks = filepath.EvalSymlinks
