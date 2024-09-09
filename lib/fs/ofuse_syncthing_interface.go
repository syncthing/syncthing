// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	ffs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/syncthing/syncthing/lib/protocol"
)

type OwnFuseFilesystemImpl struct {
	loopback_root       string
	mnt                 string
	server              *fuse.Server
	basic_fs            *BasicFilesystem
	changeNotifyChan    chan Event
	changeNotifyErrChan chan error
}

type OwnFuseFilesystem struct {
	impl *OwnFuseFilesystemImpl
}

var filesystemOFuseMap map[string]*OwnFuseFilesystem = make(map[string]*OwnFuseFilesystem)

func NewOwnFuseFilesystem(root string, opts ...Option) *OwnFuseFilesystem {

	instance, ok := filesystemOFuseMap[root]
	if ok {
		return instance
	}

	changeNotifyChan := make(chan Event, 1000)
	changeNotifyErrChan := make(chan error, 20)

	loopback_root := fmt.Sprintf("%s/.stfolder/.loopback_root", root)
	os.MkdirAll(loopback_root, 0o770)
	loopback, err := NewLoopbackRoot(loopback_root, changeNotifyChan)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	basic_fs := newBasicFilesystem(loopback_root, opts...)

	mnt := fmt.Sprintf("%s/ofuse", root)
	os.MkdirAll(mnt, 0o770)
	server, err := ffs.Mount(mnt, loopback, nil)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	fmt.Println("fuse filesystem mounted")
	fmt.Printf("to unmount: fusermount -u %s\n", mnt)

	new_instance_impl := &OwnFuseFilesystemImpl{
		loopback_root:       loopback_root,
		mnt:                 mnt,
		server:              server,
		basic_fs:            basic_fs,
		changeNotifyChan:    changeNotifyChan,
		changeNotifyErrChan: changeNotifyErrChan,
	}

	new_instance := &OwnFuseFilesystem{
		impl: new_instance_impl,
	}

	filesystemOFuseMap[root] = new_instance

	return new_instance
}

func (o OwnFuseFilesystem) Chmod(name string, mode FileMode) error {
	return o.impl.basic_fs.Chmod(name, mode)
}

// uid/gid as strings; numeric on POSIX, SID on Windows, like in os/user package
func (o OwnFuseFilesystem) Lchown(name string, uid, gid string) error {
	return o.impl.basic_fs.Lchown(name, uid, gid)
}

func (o OwnFuseFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return o.impl.basic_fs.Chtimes(name, atime, mtime)
}

func (o OwnFuseFilesystem) Create(name string) (File, error) {
	return o.impl.basic_fs.Create(name)
}
func (o OwnFuseFilesystem) CreateSymlink(target, name string) error {
	return o.impl.basic_fs.CreateSymlink(target, name)
}
func (o OwnFuseFilesystem) DirNames(name string) ([]string, error) {
	return o.impl.basic_fs.DirNames(name)
}
func (o OwnFuseFilesystem) Lstat(name string) (FileInfo, error) {
	return o.impl.basic_fs.Lstat(name)
}
func (o OwnFuseFilesystem) Mkdir(name string, perm FileMode) error {
	return o.impl.basic_fs.Mkdir(name, perm)
}
func (o OwnFuseFilesystem) MkdirAll(name string, perm FileMode) error {
	return nil
}
func (o OwnFuseFilesystem) Open(name string) (File, error) {
	file, err := o.impl.basic_fs.Open(name)
	if err != nil {
		return nil, err
	}
	return ofuseFile{
		basicFile: file.(basicFile),
		fs:        &o,
	}, nil
}

func (o OwnFuseFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	file, err := o.impl.basic_fs.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return ofuseFile{
		basicFile: file.(basicFile),
		fs:        &o,
	}, nil
}

func (o OwnFuseFilesystem) ReadSymlink(name string) (string, error) {
	return o.impl.basic_fs.ReadSymlink(name)
}
func (o OwnFuseFilesystem) Remove(name string) error {
	return o.impl.basic_fs.Remove(name)
}
func (o OwnFuseFilesystem) RemoveAll(name string) error {
	return o.impl.basic_fs.RemoveAll(name)
}
func (o OwnFuseFilesystem) Rename(oldname, newname string) error {
	return o.impl.basic_fs.Rename(oldname, newname)
}
func (o OwnFuseFilesystem) Stat(name string) (FileInfo, error) {
	return o.impl.basic_fs.Stat(name)
}
func (o OwnFuseFilesystem) SymlinksSupported() bool {
	return o.impl.basic_fs.SymlinksSupported()
}
func (o OwnFuseFilesystem) Walk(name string, walkFn WalkFunc) error {
	return o.impl.basic_fs.Walk(name, walkFn)
}

// If setup fails, returns non-nil error, and if afterwards a fatal (!)
// error occurs, sends that error on the channel. Afterwards this watch
// can be considered stopped.
func (o OwnFuseFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool,
) (<-chan Event, <-chan error, error) {
	return o.impl.changeNotifyChan, o.impl.changeNotifyErrChan, nil
}

func (o OwnFuseFilesystem) Hide(name string) error {
	return o.impl.basic_fs.Hide(name)
}
func (o OwnFuseFilesystem) Unhide(name string) error {
	return o.impl.basic_fs.Unhide(name)
}
func (o OwnFuseFilesystem) Glob(pattern string) ([]string, error) {
	return o.impl.basic_fs.Glob(pattern)
}
func (o OwnFuseFilesystem) Roots() ([]string, error) {
	return o.impl.basic_fs.Roots()
}
func (o OwnFuseFilesystem) Usage(name string) (Usage, error) {
	return o.impl.basic_fs.Usage(name)
}
func (o OwnFuseFilesystem) Type() FilesystemType {
	return o.impl.basic_fs.Type()
}
func (o OwnFuseFilesystem) URI() string {
	return o.impl.basic_fs.URI()
}
func (o OwnFuseFilesystem) Options() []Option {
	return o.impl.basic_fs.Options()
}
func (o OwnFuseFilesystem) SameFile(fi1, fi2 FileInfo) bool {
	return o.impl.basic_fs.SameFile(fi1, fi2)
}
func (o OwnFuseFilesystem) PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter,
) (protocol.PlatformData, error) {
	return o.impl.basic_fs.PlatformData(name, withOwnership, withXattrs, xattrFilter)
}
func (o OwnFuseFilesystem) GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error) {
	return o.impl.basic_fs.GetXattr(name, xattrFilter)
}
func (o OwnFuseFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error {
	return o.impl.basic_fs.SetXattr(path, xattrs, xattrFilter)
}

// Used for unwrapping things
func (o OwnFuseFilesystem) underlying() (Filesystem, bool) {
	return o.impl.basic_fs.underlying()
}
func (o OwnFuseFilesystem) wrapperType() filesystemWrapperType {
	return o.impl.basic_fs.wrapperType()
}
