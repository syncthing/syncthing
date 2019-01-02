// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"time"
)

type MustFilesystem struct {
	Filesystem
}

func must(fn func() error) {
	if err := fn(); err != nil {
		panic(err)
	}
}

func (fs *MustFilesystem) Chmod(name string, mode FileMode) error {
	must(func() error { return fs.Filesystem.Chmod(name, mode) })
	return nil
}

func (fs *MustFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	must(func() error { return fs.Filesystem.Chtimes(name, atime, mtime) })
	return nil
}

func (fs *MustFilesystem) Create(name string) (File, error) {
	file, err := fs.Filesystem.Create(name)
	if err != nil {
		panic(err)
	}
	return file, nil
}

func (fs *MustFilesystem) CreateSymlink(target, name string) error {
	must(func() error { return fs.Filesystem.CreateSymlink(target, name) })
	return nil
}

func (fs *MustFilesystem) DirNames(name string) ([]string, error) {
	names, err := fs.Filesystem.DirNames(name)
	if err != nil {
		panic(err)
	}
	return names, nil
}

func (fs *MustFilesystem) Lstat(name string) (FileInfo, error) {
	info, err := fs.Filesystem.Lstat(name)
	if err != nil {
		panic(err)
	}
	return info, nil
}

func (fs *MustFilesystem) Mkdir(name string, perm FileMode) error {
	must(func() error { return fs.Filesystem.Mkdir(name, perm) })
	return nil
}

func (fs *MustFilesystem) MkdirAll(name string, perm FileMode) error {
	must(func() error { return fs.Filesystem.MkdirAll(name, perm) })
	return nil
}

func (fs *MustFilesystem) Open(name string) (File, error) {
	file, err := fs.Filesystem.Open(name)
	if err != nil {
		panic(err)
	}
	return file, nil
}

func (fs *MustFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	file, err := fs.Filesystem.OpenFile(name, flags, mode)
	if err != nil {
		panic(err)
	}
	return file, nil
}

func (fs *MustFilesystem) ReadSymlink(name string) (string, error) {
	target, err := fs.Filesystem.ReadSymlink(name)
	if err != nil {
		panic(err)
	}
	return target, nil
}

func (fs *MustFilesystem) Remove(name string) error {
	if err := fs.Filesystem.Remove(name); err != nil && !IsNotExist(err) {
		panic(err)
	}
	return nil
}

func (fs *MustFilesystem) RemoveAll(name string) error {
	if err := fs.Filesystem.RemoveAll(name); err != nil && !IsNotExist(err) {
		panic(err)
	}
	return nil
}

func (fs *MustFilesystem) Rename(oldname, newname string) error {
	must(func() error { return fs.Filesystem.Rename(oldname, newname) })
	return nil
}

func (fs *MustFilesystem) Stat(name string) (FileInfo, error) {
	info, err := fs.Filesystem.Stat(name)
	if err != nil {
		panic(err)
	}
	return info, nil
}

func (fs *MustFilesystem) SymlinksSupported() bool {
	return fs.Filesystem.SymlinksSupported()
}

func (fs *MustFilesystem) Walk(root string, walkFn WalkFunc) error {
	must(func() error { return fs.Filesystem.Walk(root, walkFn) })
	return nil
}

func (fs *MustFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, error) {
	evChan, err := fs.Filesystem.Watch(path, ignore, ctx, ignorePerms)
	if err != nil {
		panic(err)
	}
	return evChan, nil
}

func (fs *MustFilesystem) Unhide(name string) error {
	must(func() error { return fs.Filesystem.Unhide(name) })
	return nil
}

func (fs *MustFilesystem) Hide(name string) error {
	must(func() error { return fs.Filesystem.Hide(name) })
	return nil
}

func (fs *MustFilesystem) Glob(name string) ([]string, error) {
	names, err := fs.Filesystem.Glob(name)
	if err != nil {
		panic(err)
	}
	return names, nil
}

func (fs *MustFilesystem) Roots() ([]string, error) {
	roots, err := fs.Filesystem.Roots()
	if err != nil {
		panic(err)
	}
	return roots, nil
}

func (fs *MustFilesystem) Usage(name string) (Usage, error) {
	usage, err := fs.Filesystem.Usage(name)
	if err != nil {
		panic(err)
	}
	return usage, nil
}
