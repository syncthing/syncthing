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

type errorFilesystem struct {
	err    error
	fsType FilesystemType
	uri    string
}

func (fs *errorFilesystem) Chmod(name string, mode FileMode) error { return fs.err }
func (fs *errorFilesystem) Lchown(name string, uid, gid int) error { return fs.err }
func (fs *errorFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fs.err
}
func (fs *errorFilesystem) Create(name string) (File, error)             { return nil, fs.err }
func (fs *errorFilesystem) CreateSymlink(target, name string) error      { return fs.err }
func (fs *errorFilesystem) DirNames(name string) ([]string, error)       { return nil, fs.err }
func (fs *errorFilesystem) Lstat(name string) (FileInfo, error)          { return nil, fs.err }
func (fs *errorFilesystem) Mkdir(name string, perm FileMode) error       { return fs.err }
func (fs *errorFilesystem) MkdirAll(name string, perm FileMode) error    { return fs.err }
func (fs *errorFilesystem) Open(name string) (File, error)               { return nil, fs.err }
func (fs *errorFilesystem) OpenFile(string, int, FileMode) (File, error) { return nil, fs.err }
func (fs *errorFilesystem) ReadSymlink(name string) (string, error)      { return "", fs.err }
func (fs *errorFilesystem) Remove(name string) error                     { return fs.err }
func (fs *errorFilesystem) RemoveAll(name string) error                  { return fs.err }
func (fs *errorFilesystem) Rename(oldname, newname string) error         { return fs.err }
func (fs *errorFilesystem) Stat(name string) (FileInfo, error)           { return nil, fs.err }
func (fs *errorFilesystem) SymlinksSupported() bool                      { return false }
func (fs *errorFilesystem) Walk(root string, walkFn WalkFunc) error      { return fs.err }
func (fs *errorFilesystem) Unhide(name string) error                     { return fs.err }
func (fs *errorFilesystem) Hide(name string) error                       { return fs.err }
func (fs *errorFilesystem) Glob(pattern string) ([]string, error)        { return nil, fs.err }
func (fs *errorFilesystem) SyncDir(name string) error                    { return fs.err }
func (fs *errorFilesystem) Roots() ([]string, error)                     { return nil, fs.err }
func (fs *errorFilesystem) Usage(name string) (Usage, error)             { return Usage{}, fs.err }
func (fs *errorFilesystem) Type() FilesystemType                         { return fs.fsType }
func (fs *errorFilesystem) URI() string                                  { return fs.uri }
func (fs *errorFilesystem) Options() []Option {
	return nil
}
func (fs *errorFilesystem) SameFile(fi1, fi2 FileInfo) bool { return false }
func (fs *errorFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	return nil, nil, fs.err
}

func (fs *errorFilesystem) underlying() (Filesystem, bool) {
	return nil, false
}

func (fs *errorFilesystem) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeError
}
