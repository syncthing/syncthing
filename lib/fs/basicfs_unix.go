// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func (*BasicFilesystem) SymlinksSupported() bool {
	return true
}

func (f *BasicFilesystem) CreateSymlink(target, name string) error {
	name2, err := f.rooted(name, "symlink")
	if err != nil {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return &os.LinkError{Op: "symlink", Old: target, New: name, Err: syscall.ENOENT}
		}
		return err
	}
	_, err = f.rooted(target, "symlink")
	if err != nil {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return &os.LinkError{Op: "symlink", Old: target, New: name, Err: syscall.ENOENT}
		}
		// Currently we allow targets to live outside of f.root, so
		// we ignore these errors
	}
	if f.encoder != nil {
		target = f.encoder.encode(target)
	}
	return os.Symlink(target, name2)
}

func (f *BasicFilesystem) ReadSymlink(name string) (string, error) {
	name, err := f.rooted(name, "readlink")
	if err != nil {
		return "", err
	}
	res, err := os.Readlink(name)
	if f.encoder != nil {
		res = decode(res)
	}
	return res, err
}

func (*BasicFilesystem) mkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Unhide is a noop on unix, as unhiding files requires renaming them.
// We still check that the relative path does not try to escape the root
func (f *BasicFilesystem) Unhide(name string) error {
	_, err := f.rooted(name, "unhide")
	return err
}

// Hide is a noop on unix, as hiding files requires renaming them.
// We still check that the relative path does not try to escape the root
func (f *BasicFilesystem) Hide(name string) error {
	_, err := f.rooted(name, "hide")
	return err
}

func (*BasicFilesystem) Roots() ([]string, error) {
	return []string{"/"}, nil
}

func (f *BasicFilesystem) Lchown(name, uid, gid string) error {
	name, err := f.rooted(name, "lchown")
	if err != nil {
		return err
	}
	nuid, err := strconv.Atoi(uid)
	if err != nil {
		return err
	}
	ngid, err := strconv.Atoi(gid)
	if err != nil {
		return err
	}
	return os.Lchown(name, nuid, ngid)
}

func (f *BasicFilesystem) Remove(name string) error {
	name, err := f.rooted(name, "remove")
	if err != nil {
		return err
	}
	return os.Remove(name)
}

// unrootedChecked returns the path relative to the folder root (same as
// unrooted) or an error if the given path is not a subpath and handles the
// special case when the given path is the folder root without a trailing
// pathseparator.
func (f *BasicFilesystem) unrootedChecked(absPath string, roots []string) (string, *ErrWatchEventOutsideRoot) {
	for _, root := range roots {
		// Make sure the root ends with precisely one path separator, to
		// ease prefix comparisons.
		root := strings.TrimRight(root, string(PathSeparator)) + string(PathSeparator)

		if absPath+string(PathSeparator) == root {
			return ".", nil
		}
		if strings.HasPrefix(absPath, root) {
			return rel(absPath, root), nil
		}
	}
	return "", f.newErrWatchEventOutsideRoot(absPath, roots)
}

func rel(path, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, prefix), string(PathSeparator))
}

var evalSymlinks = filepath.EvalSymlinks

// watchPaths adjust the folder root for use with the notify backend and the
// corresponding absolute path to be passed to notify to watch name.
func (f *BasicFilesystem) watchPaths(name string) (string, []string, error) {
	root, err := evalSymlinks(f.root)
	if err != nil {
		return "", nil, err
	}

	absName, err := rooted(name, root)
	if err != nil {
		return "", nil, err
	}

	return filepath.Join(absName, "..."), []string{root}, nil
}
