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

type RealFuseFilesystemImpl struct {
	loopback_root       string
	mnt                 string
	server              *fuse.Server
	basic_fs            *BasicFilesystem
	changeNotifyChan    chan Event
	changeNotifyErrChan chan error
}

type RealFuseFilesystem struct {
	impl *RealFuseFilesystemImpl
}

var filesystemRFuseMap map[string]*RealFuseFilesystem = make(map[string]*RealFuseFilesystem)

func NewRealFuseFilesystem(root string, opts ...Option) *RealFuseFilesystem {

	instance, ok := filesystemRFuseMap[root]
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

	mnt := fmt.Sprintf("%s/rfuse", root)
	os.MkdirAll(mnt, 0o770)
	server, err := ffs.Mount(mnt, loopback, nil)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	fmt.Println("fuse filesystem mounted")
	fmt.Printf("to unmount: fusermount -u %s\n", mnt)

	new_instance_impl := &RealFuseFilesystemImpl{
		loopback_root:       loopback_root,
		mnt:                 mnt,
		server:              server,
		basic_fs:            basic_fs,
		changeNotifyChan:    changeNotifyChan,
		changeNotifyErrChan: changeNotifyErrChan,
	}

	new_instance := &RealFuseFilesystem{
		impl: new_instance_impl,
	}

	filesystemRFuseMap[root] = new_instance

	return new_instance
}

func (o RealFuseFilesystem) Chmod(name string, mode FileMode) error {
	return o.impl.basic_fs.Chmod(name, mode)
}

// uid/gid as strings; numeric on POSIX, SID on Windows, like in os/user package
func (o RealFuseFilesystem) Lchown(name string, uid, gid string) error {
	return o.impl.basic_fs.Lchown(name, uid, gid)
}

func (o RealFuseFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return o.impl.basic_fs.Chtimes(name, atime, mtime)
}

func (o RealFuseFilesystem) Create(name string) (File, error) {
	return o.impl.basic_fs.Create(name)
}
func (o RealFuseFilesystem) CreateSymlink(target, name string) error {
	return o.impl.basic_fs.CreateSymlink(target, name)
}
func (o RealFuseFilesystem) DirNames(name string) ([]string, error) {
	return o.impl.basic_fs.DirNames(name)
}
func (o RealFuseFilesystem) Lstat(name string) (FileInfo, error) {
	return o.impl.basic_fs.Lstat(name)
}
func (o RealFuseFilesystem) Mkdir(name string, perm FileMode) error {
	return o.impl.basic_fs.Mkdir(name, perm)
}
func (o RealFuseFilesystem) MkdirAll(name string, perm FileMode) error {
	return nil
}
func (o RealFuseFilesystem) Open(name string) (File, error) {
	file, err := o.impl.basic_fs.Open(name)
	if err != nil {
		return nil, err
	}
	return rfuseFile{
		basicFile: file.(basicFile),
		fs:        &o,
	}, nil
}

func (o RealFuseFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	file, err := o.impl.basic_fs.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return rfuseFile{
		basicFile: file.(basicFile),
		fs:        &o,
	}, nil
}

func (o RealFuseFilesystem) ReadSymlink(name string) (string, error) {
	return o.impl.basic_fs.ReadSymlink(name)
}
func (o RealFuseFilesystem) Remove(name string) error {
	return o.impl.basic_fs.Remove(name)
}
func (o RealFuseFilesystem) RemoveAll(name string) error {
	return o.impl.basic_fs.RemoveAll(name)
}
func (o RealFuseFilesystem) Rename(oldname, newname string) error {
	return o.impl.basic_fs.Rename(oldname, newname)
}
func (o RealFuseFilesystem) Stat(name string) (FileInfo, error) {
	return o.impl.basic_fs.Stat(name)
}
func (o RealFuseFilesystem) SymlinksSupported() bool {
	return o.impl.basic_fs.SymlinksSupported()
}
func (o RealFuseFilesystem) Walk(name string, walkFn WalkFunc) error {
	return o.impl.basic_fs.Walk(name, walkFn)
}

// If setup fails, returns non-nil error, and if afterwards a fatal (!)
// error occurs, sends that error on the channel. Afterwards this watch
// can be considered stopped.
func (o RealFuseFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool,
) (<-chan Event, <-chan error, error) {
	return o.impl.changeNotifyChan, o.impl.changeNotifyErrChan, nil
}

func (o RealFuseFilesystem) Hide(name string) error {
	return o.impl.basic_fs.Hide(name)
}
func (o RealFuseFilesystem) Unhide(name string) error {
	return o.impl.basic_fs.Unhide(name)
}
func (o RealFuseFilesystem) Glob(pattern string) ([]string, error) {
	return o.impl.basic_fs.Glob(pattern)
}
func (o RealFuseFilesystem) Roots() ([]string, error) {
	return o.impl.basic_fs.Roots()
}
func (o RealFuseFilesystem) Usage(name string) (Usage, error) {
	return o.impl.basic_fs.Usage(name)
}
func (o RealFuseFilesystem) Type() FilesystemType {
	return o.impl.basic_fs.Type()
}
func (o RealFuseFilesystem) URI() string {
	return o.impl.basic_fs.URI()
}
func (o RealFuseFilesystem) Options() []Option {
	return o.impl.basic_fs.Options()
}
func (o RealFuseFilesystem) SameFile(fi1, fi2 FileInfo) bool {
	return o.impl.basic_fs.SameFile(fi1, fi2)
}
func (o RealFuseFilesystem) PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter,
) (protocol.PlatformData, error) {
	return o.impl.basic_fs.PlatformData(name, withOwnership, withXattrs, xattrFilter)
}
func (o RealFuseFilesystem) GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error) {
	return o.impl.basic_fs.GetXattr(name, xattrFilter)
}
func (o RealFuseFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error {
	return o.impl.basic_fs.SetXattr(path, xattrs, xattrFilter)
}

// Used for unwrapping things
func (o RealFuseFilesystem) underlying() (Filesystem, bool) {
	return o.impl.basic_fs.underlying()
}
func (o RealFuseFilesystem) wrapperType() filesystemWrapperType {
	return o.impl.basic_fs.wrapperType()
}
