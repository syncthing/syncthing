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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
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

func (f *BasicFilesystem) GetXattr(path string) (map[string][]byte, error) {
	path, err := f.rooted(path)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 1)
	buf, err = listXattr(path, buf)
	if err != nil {
		return nil, err
	}

	attrs := strings.Split(string(buf), "\x00")
	res := make(map[string][]byte, len(attrs))
	var val []byte
	for _, attr := range attrs {
		if attr == "" {
			continue
		}
		val, buf, err = getXattr(path, attr, buf)
		if err != nil {
			fmt.Println("Error getting xattr", attr, err)
			continue
		}
		res[attr] = val
	}
	return res, nil
}

func listXattr(path string, buf []byte) ([]byte, error) {
	size, err := unix.Listxattr(path, buf)
	if errors.Is(err, syscall.ERANGE) {
		// Buffer is too small. Try again with a zero sized buffer to get
		// the size, then allocate a buffer of the correct size.
		size, err = unix.Listxattr(path, nil)
		if err != nil {
			return nil, err
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Listxattr(path, buf)
	}
	return buf, err
}

func getXattr(path, name string, buf []byte) (val []byte, rest []byte, err error) {
	if len(buf) == 0 {
		buf = make([]byte, 1024)
	}
	size, err := unix.Getxattr(path, name, buf)
	if errors.Is(err, syscall.ERANGE) {
		// Buffer was too small. Figure out how large it needs to be, and
		// allocate.
		size, err = unix.Getxattr(path, name, nil)
		if err != nil {
			return nil, nil, err
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Getxattr(path, name, buf)
	}
	if err != nil {
		return nil, buf, err
	}
	return buf[:size], buf[size:], nil
}

func (f *BasicFilesystem) SetXattr(path, key string, val []byte) error {
	path, err := f.rooted(path)
	if err != nil {
		return err
	}
	return unix.Setxattr(path, key, val, 0)
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
