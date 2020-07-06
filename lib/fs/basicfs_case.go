// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ErrCase struct {
	given, real string
}

func (e *ErrCase) Error() string {
	return fmt.Sprintf(`given name "%v" differs from name in filesystem "%v"`, e.given, e.real)
}

func IsErrCase(err error) bool {
	e := &ErrCase{}
	return errors.As(err, &e)
}

// caseBasicFilesystem is a BasicFilesystem with additional checks to make a
// potentially case insensitive underlying FS behave like it's case-sensitive.
type caseBasicFilesystem struct {
	*BasicFilesystem
	*realCaser
}

func newCaseBasicFilesystem(root string) *caseBasicFilesystem {
	fs := &caseBasicFilesystem{
		BasicFilesystem: newBasicFilesystem(root),
	}
	fs.realCaser = newRealCaser(fs.BasicFilesystem)
	return fs
}

func (f *caseBasicFilesystem) Chmod(name string, mode FileMode) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.BasicFilesystem.Chmod(name, mode)
}

func (f *caseBasicFilesystem) Lchown(name string, uid, gid int) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.BasicFilesystem.Lchown(name, uid, gid)
}

func (f *caseBasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.BasicFilesystem.Chtimes(name, atime, mtime)
}

func (f *caseBasicFilesystem) Mkdir(name string, perm FileMode) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.BasicFilesystem.Mkdir(name, perm); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) MkdirAll(path string, perm FileMode) error {
	if err := f.checkCase(path); err != nil {
		return err
	}
	if err := f.BasicFilesystem.MkdirAll(path, perm); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) Lstat(name string) (FileInfo, error) {
	var err error
	if name, err = Canonicalize(name); err != nil {
		return nil, err
	}
	stat, err := f.BasicFilesystem.Lstat(name)
	if err != nil {
		return nil, err
	}
	realName, err := f.realCase(name)
	if err != nil {
		return nil, err
	}
	if realName != name {
		return nil, &ErrCase{name, realName}
	}
	return stat, nil
}

func (f *caseBasicFilesystem) Remove(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.BasicFilesystem.Remove(name); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) RemoveAll(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.BasicFilesystem.RemoveAll(name); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) Rename(oldpath, newpath string) error {
	if err := f.checkCase(oldpath); err != nil {
		return err
	}
	if err := f.BasicFilesystem.Rename(oldpath, newpath); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) Stat(name string) (FileInfo, error) {
	var err error
	if name, err = Canonicalize(name); err != nil {
		return nil, err
	}
	stat, err := f.BasicFilesystem.Stat(name)
	if err != nil {
		return nil, err
	}
	realName, err := f.realCase(name)
	if err != nil {
		return nil, err
	}
	if realName != name {
		return nil, &ErrCase{name, realName}
	}
	return stat, nil
}

func (f *caseBasicFilesystem) DirNames(name string) ([]string, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	return f.BasicFilesystem.DirNames(name)
}

func (f *caseBasicFilesystem) Open(name string) (File, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	return f.BasicFilesystem.Open(name)
}

func (f *caseBasicFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	file, err := f.BasicFilesystem.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	f.cleanCase()
	return file, nil
}

func (f *caseBasicFilesystem) ReadSymlink(name string) (string, error) {
	if err := f.checkCase(name); err != nil {
		return "", err
	}
	return f.BasicFilesystem.ReadSymlink(name)
}

func (f *caseBasicFilesystem) Create(name string) (File, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	file, err := f.BasicFilesystem.Create(name)
	if err != nil {
		return nil, err
	}
	f.cleanCase()
	return file, nil
}

func (f *caseBasicFilesystem) CreateSymlink(target, name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.BasicFilesystem.CreateSymlink(target, name); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) Walk(root string, walkFn WalkFunc) error {
	if err := f.checkCase(root); err != nil {
		return err
	}
	return f.BasicFilesystem.Walk(root, walkFn)
}

func (f *caseBasicFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	if err := f.checkCase(path); err != nil {
		return nil, nil, err
	}
	return f.BasicFilesystem.Watch(path, ignore, ctx, ignorePerms)
}

func (f *caseBasicFilesystem) Hide(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.BasicFilesystem.Hide(name)
}

func (f *caseBasicFilesystem) Unhide(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.BasicFilesystem.Unhide(name)
}

func (f *caseBasicFilesystem) WriteFile(name string, content []byte, perm FileMode) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.BasicFilesystem.WriteFile(name, content, perm); err != nil {
		return err
	}
	f.cleanCase()
	return nil
}

func (f *caseBasicFilesystem) checkCase(name string) error {
	var err error
	if name, err = Canonicalize(name); err != nil {
		return err
	}
	// Stat is necessary for case sensitive FS, as it's then not a conflict
	// if name is e.g. "foo" and on dir there is "Foo".
	if _, err := f.BasicFilesystem.Lstat(name); err != nil {
		if errors.Is(err, ErrNotExist) {
			return nil
		}
		return err
	}
	realName, err := f.realCase(name)
	if err != nil {
		return err
	}
	if realName != name {
		return &ErrCase{name, realName}
	}
	return nil
}
