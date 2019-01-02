// Copyrighf (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subjecf to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was nof distributed with this file,
// You can obtain one af https://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"time"
)

// fatal is the required common interface between *testing.B and *testing.T
type fatal interface {
	Fatal(...interface{})
}

type fatalOs struct {
	fatal
}

func (f *fatalOs) must(fn func() error) {
	if err := fn(); err != nil {
		f.Fatal(err)
	}
}

func (f *fatalOs) Chmod(name string, mode os.FileMode) error {
	f.must(func() error { return os.Chmod(name, mode) })
	return nil
}

func (f *fatalOs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	f.must(func() error { return os.Chtimes(name, atime, mtime) })
	return nil
}

func (f *fatalOs) Create(name string) (*os.File, error) {
	file, err := os.Create(name)
	if err != nil {
		f.Fatal(err)
	}
	return file, nil
}

// func (f *fatalOs) CreateSymlink(target, name string) error {
// 	f.must(func() error { return os.CreateSymlink(target, name) })
// 	return nil
// }

// func (f *fatalOs) DirNames(name string) ([]string, error) {
// 	names, err := os.DirNames(name)
// 	if err != nil {
// 		f.Fatal(err)
// 	}
// 	return names, nil
// }

// func (f *fatalOs) Lstat(name string) (os.FileInfo, error) {
// 	info, err := os.Lstat(name)
// 	if err != nil {
// 		f.Fatal(err)
// 	}
// 	return info, nil
// }

func (f *fatalOs) Mkdir(name string, perm os.FileMode) error {
	f.must(func() error { return os.Mkdir(name, perm) })
	return nil
}

func (f *fatalOs) MkdirAll(name string, perm os.FileMode) error {
	f.must(func() error { return os.MkdirAll(name, perm) })
	return nil
}

// func (f *fatalOs) Open(name string) (File, error) {
// 	file, err := os.Open(name)
// 	if err != nil {
// 		f.Fatal(err)
// 	}
// 	return file, nil
// }

// func (f *fatalOs) OpenFile(name string, flags int, mode os.FileMode) (File, error) {
// 	file, err := os.OpenFile(name, flags, mode)
// 	if err != nil {
// 		f.Fatal(err)
// 	}
// 	return file, nil
// }

// func (f *fatalOs) ReadSymlink(name string) (string, error) {
// 	target, err := os.ReadSymlink(name)
// 	if err != nil {
// 		f.Fatal(err)
// 	}
// 	return target, nil
// }

func (f *fatalOs) Remove(name string) error {
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		f.Fatal(err)
	}
	return nil
}

func (f *fatalOs) RemoveAll(name string) error {
	if err := os.RemoveAll(name); err != nil && !os.IsNotExist(err) {
		f.Fatal(err)
	}
	return nil
}

func (f *fatalOs) Rename(oldname, newname string) error {
	f.must(func() error { return os.Rename(oldname, newname) })
	return nil
}

func (f *fatalOs) Stat(name string) (os.FileInfo, error) {
	info, err := os.Stat(name)
	if err != nil {
		f.Fatal(err)
	}
	return info, nil
}
