// Copyright (C) 2024 The Syncthing Authors.
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

type ExtFilesystemImpl interface {
	Chmod(name string, mode FileMode) error
	Lchown(name string, uid, gid string) error // uid/gid as strings; numeric on POSIX, SID on Windows, like in os/user package
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Create(name string) (File, error)
	CreateSymlink(target, name string) error
	DirNames(name string) ([]string, error)
	Lstat(name string) (FileInfo, error)
	Mkdir(name string, perm FileMode) error
	MkdirAll(name string, perm FileMode) error
	Open(name string) (File, error)
	OpenFile(name string, flags int, mode FileMode) (File, error)
	ReadSymlink(name string) (string, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname, newname string) error
	Stat(name string) (FileInfo, error)
	SymlinksSupported() bool
	Walk(name string, walkFn WalkFunc) error
	Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error)
	Hide(name string) error
	Unhide(name string) error
	Glob(pattern string) ([]string, error)
	Roots() ([]string, error)
	Usage(name string) (Usage, error)
	Type() FilesystemType
	URI() string
	Options() []Option
	SameFile(fi1, fi2 FileInfo) bool
	PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error)
	GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error)
	SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error
}

type ExtFilesystem struct {
	impl ExtFilesystemImpl
}

func NewExtFilesystem(impl ExtFilesystemImpl) *ExtFilesystem {
	return &ExtFilesystem{
		impl: impl,
	}
}

func (fs *ExtFilesystem) Chmod(name string, mode FileMode) error {
	return fs.impl.Chmod(name, mode)
}

func (fs *ExtFilesystem) Lchown(name string, uid, gid string) error {
	return fs.impl.Lchown(name, uid, gid)
}

func (fs *ExtFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fs.impl.Chtimes(name, atime, mtime)
}

func (fs *ExtFilesystem) Create(name string) (File, error) {
	return fs.impl.Create(name)
}

func (fs *ExtFilesystem) CreateSymlink(target, name string) error {
	return fs.impl.CreateSymlink(target, name)
}

func (fs *ExtFilesystem) DirNames(name string) ([]string, error) {
	return fs.impl.DirNames(name)
}

func (fs *ExtFilesystem) Lstat(name string) (FileInfo, error) {
	return fs.impl.Lstat(name)
}

func (fs *ExtFilesystem) Mkdir(name string, perm FileMode) error {
	return fs.impl.Mkdir(name, perm)
}

func (fs *ExtFilesystem) MkdirAll(name string, perm FileMode) error {
	return fs.impl.MkdirAll(name, perm)
}

func (fs *ExtFilesystem) Open(name string) (File, error) {
	return fs.impl.Open(name)
}

func (fs *ExtFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	return fs.impl.OpenFile(name, flags, mode)
}

func (fs *ExtFilesystem) ReadSymlink(name string) (string, error) {
	return fs.impl.ReadSymlink(name)
}

func (fs *ExtFilesystem) Remove(name string) error {
	return fs.impl.Remove(name)
}

func (fs *ExtFilesystem) RemoveAll(name string) error {
	return fs.impl.RemoveAll(name)
}

func (fs *ExtFilesystem) Rename(oldname, newname string) error {
	return fs.impl.Rename(oldname, newname)
}

func (fs *ExtFilesystem) Stat(name string) (FileInfo, error) {
	return fs.impl.Stat(name)
}

func (fs *ExtFilesystem) SymlinksSupported() bool {
	return fs.impl.SymlinksSupported()
}

func (fs *ExtFilesystem) Walk(name string, walkFn WalkFunc) error {
	return fs.impl.Walk(name, walkFn)
}

func (fs *ExtFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	return fs.impl.Watch(path, ignore, ctx, ignorePerms)
}

func (fs *ExtFilesystem) Hide(name string) error {
	return fs.impl.Hide(name)
}

func (fs *ExtFilesystem) Unhide(name string) error {
	return fs.impl.Unhide(name)
}

func (fs *ExtFilesystem) Glob(pattern string) ([]string, error) {
	return fs.impl.Glob(pattern)
}

func (fs *ExtFilesystem) Roots() ([]string, error) {
	return fs.impl.Roots()
}

func (fs *ExtFilesystem) Usage(name string) (Usage, error) {
	return fs.impl.Usage(name)
}

func (fs *ExtFilesystem) Type() FilesystemType {
	return fs.impl.Type()
}

func (fs *ExtFilesystem) URI() string {
	return fs.impl.URI()
}

func (fs *ExtFilesystem) Options() []Option {
	return fs.impl.Options()
}

func (fs *ExtFilesystem) SameFile(fi1, fi2 FileInfo) bool {
	return fs.impl.SameFile(fi1, fi2)
}

func (fs *ExtFilesystem) PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error) {
	return fs.impl.PlatformData(name, withOwnership, withXattrs, xattrFilter)
}

func (fs *ExtFilesystem) GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error) {
	return fs.impl.GetXattr(name, xattrFilter)
}

func (fs *ExtFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error {
	return fs.impl.SetXattr(path, xattrs, xattrFilter)
}

func (*ExtFilesystem) underlying() (Filesystem, bool) {
	return nil, false
}

func (*ExtFilesystem) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeError
}
