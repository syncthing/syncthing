// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

// fatal is the required common interface between *testing.B and *testing.T
type fatal interface {
	Fatal(...interface{})
	Helper()
}

type fatalOs struct {
	fatal
}

func must(f fatal, err error) {
	f.Helper()
	if err != nil {
		f.Fatal(err)
	}
}

func mustRemove(f fatal, err error) {
	f.Helper()
	if err != nil && !fs.IsNotExist(err) {
		f.Fatal(err)
	}
}

func (f *fatalOs) Chmod(name string, mode os.FileMode) {
	f.Helper()
	must(f, os.Chmod(name, mode))
}

func (f *fatalOs) Chtimes(name string, atime time.Time, mtime time.Time) {
	f.Helper()
	must(f, os.Chtimes(name, atime, mtime))
}

func (f *fatalOs) Create(name string) *os.File {
	f.Helper()
	file, err := os.Create(name)
	must(f, err)
	return file
}

func (f *fatalOs) Mkdir(name string, perm os.FileMode) {
	f.Helper()
	must(f, os.Mkdir(name, perm))
}

func (f *fatalOs) MkdirAll(name string, perm os.FileMode) {
	f.Helper()
	must(f, os.MkdirAll(name, perm))
}

func (f *fatalOs) Remove(name string) {
	f.Helper()
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		f.Fatal(err)
	}
}

func (f *fatalOs) RemoveAll(name string) {
	f.Helper()
	if err := os.RemoveAll(name); err != nil && !os.IsNotExist(err) {
		f.Fatal(err)
	}
}

func (f *fatalOs) Rename(oldname, newname string) {
	f.Helper()
	must(f, os.Rename(oldname, newname))
}

func (f *fatalOs) Stat(name string) os.FileInfo {
	f.Helper()
	info, err := os.Stat(name)
	must(f, err)
	return info
}
