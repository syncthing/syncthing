// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type errorFilesystem struct {
	err    error
	fsType FilesystemType
	uri    string
}

func (fs *errorFilesystem) Chmod(_ string, _ FileMode) error { return fs.err }
func (fs *errorFilesystem) Lchown(_, _, _ string) error      { return fs.err }
func (fs *errorFilesystem) Chtimes(_ string, _ time.Time, _ time.Time) error {
	return fs.err
}
func (fs *errorFilesystem) Create(_ string) (File, error)       { return nil, fs.err }
func (fs *errorFilesystem) CreateSymlink(_, _ string) error     { return fs.err }
func (fs *errorFilesystem) DirNames(_ string) ([]string, error) { return nil, fs.err }
func (fs *errorFilesystem) GetXattr(_ string, _ XattrFilter) ([]protocol.Xattr, error) {
	return nil, fs.err
}

func (fs *errorFilesystem) SetXattr(_ string, _ []protocol.Xattr, _ XattrFilter) error {
	return fs.err
}
func (fs *errorFilesystem) Lstat(_ string) (FileInfo, error)             { return nil, fs.err }
func (fs *errorFilesystem) Mkdir(_ string, _ FileMode) error             { return fs.err }
func (fs *errorFilesystem) MkdirAll(_ string, _ FileMode) error          { return fs.err }
func (fs *errorFilesystem) Open(_ string) (File, error)                  { return nil, fs.err }
func (fs *errorFilesystem) OpenFile(string, int, FileMode) (File, error) { return nil, fs.err }
func (fs *errorFilesystem) ReadSymlink(_ string) (string, error)         { return "", fs.err }
func (fs *errorFilesystem) Remove(_ string) error                        { return fs.err }
func (fs *errorFilesystem) RemoveAll(_ string) error                     { return fs.err }
func (fs *errorFilesystem) Rename(_, _ string) error                     { return fs.err }
func (fs *errorFilesystem) Stat(_ string) (FileInfo, error)              { return nil, fs.err }
func (*errorFilesystem) SymlinksSupported() bool                         { return false }
func (fs *errorFilesystem) Walk(_ string, _ WalkFunc) error              { return fs.err }
func (fs *errorFilesystem) Unhide(_ string) error                        { return fs.err }
func (fs *errorFilesystem) Hide(_ string) error                          { return fs.err }
func (fs *errorFilesystem) Glob(_ string) ([]string, error)              { return nil, fs.err }
func (fs *errorFilesystem) SyncDir(_ string) error                       { return fs.err }
func (fs *errorFilesystem) Roots() ([]string, error)                     { return nil, fs.err }
func (fs *errorFilesystem) Usage(_ string) (Usage, error)                { return Usage{}, fs.err }
func (fs *errorFilesystem) Type() FilesystemType                         { return fs.fsType }
func (fs *errorFilesystem) URI() string                                  { return fs.uri }
func (*errorFilesystem) Options() []Option {
	return nil
}
func (*errorFilesystem) SameFile(_, _ FileInfo) bool { return false }
func (fs *errorFilesystem) Watch(_ string, _ Matcher, _ context.Context, _ bool) (<-chan Event, <-chan error, error) {
	return nil, nil, fs.err
}

func (fs *errorFilesystem) PlatformData(_ string, _, _ bool, _ XattrFilter) (protocol.PlatformData, error) {
	return protocol.PlatformData{}, fs.err
}

func (*errorFilesystem) underlying() (Filesystem, bool) {
	return nil, false
}

func (*errorFilesystem) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeError
}
