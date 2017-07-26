// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package fs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var errNotSupported = errors.New("symlinks not supported")

func (BasicFilesystem) SymlinksSupported() bool {
	return false
}

func (BasicFilesystem) ReadSymlink(path string) (string, error) {
	return "", errNotSupported
}

func (BasicFilesystem) CreateSymlink(path, target string) error {
	return errNotSupported
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error.
// The permission bits perm are used for all directories that MkdirAll creates.
// If path is already a directory, MkdirAll does nothing and returns nil.
func (f *BasicFilesystem) MkdirAll(path string, perm FileMode) error {
	path, err := f.rooted(path)
	if err != nil {
		return err
	}

	return f.mkdirAll(path, os.FileMode(perm))
}

// Required due to https://github.com/golang/go/issues/10900
func (f *BasicFilesystem) mkdirAll(path string, perm os.FileMode) error {
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := os.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{
			Op:   "mkdir",
			Path: path,
			Err:  syscall.ENOTDIR,
		}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && IsPathSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !IsPathSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		parent := path[0 : j-1]
		if parent != filepath.VolumeName(parent) {
			err = os.MkdirAll(parent, perm)
			if err != nil {
				return err
			}
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = os.Mkdir(path, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := os.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

func (f *BasicFilesystem) Unhide(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}

	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}

	attrs &^= syscall.FILE_ATTRIBUTE_HIDDEN
	return syscall.SetFileAttributes(p, attrs)
}

func (f *BasicFilesystem) Hide(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}

	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}

	attrs |= syscall.FILE_ATTRIBUTE_HIDDEN
	return syscall.SetFileAttributes(p, attrs)
}

func (f *BasicFilesystem) Roots() ([]string, error) {
	kernel32, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return nil, err
	}
	getLogicalDriveStringsHandle, err := kernel32.FindProc("GetLogicalDriveStringsA")
	if err != nil {
		return nil, err
	}

	buffer := [1024]byte{}
	bufferSize := uint32(len(buffer))

	hr, _, _ := getLogicalDriveStringsHandle.Call(uintptr(unsafe.Pointer(&bufferSize)), uintptr(unsafe.Pointer(&buffer)))
	if hr == 0 {
		return nil, fmt.Errorf("Syscall failed")
	}

	var drives []string
	parts := bytes.Split(buffer[:], []byte{0})
	for _, part := range parts {
		if len(part) == 0 {
			break
		}
		drives = append(drives, string(part))
	}

	return drives, nil
}
