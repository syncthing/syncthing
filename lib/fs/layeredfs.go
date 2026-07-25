// Copyright (C) 2026 The Syncthing Authors.
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

const FilesystemTypeLayered FilesystemType = "layered"

// NewLayeredFilesystem stacks several filesystems on top of one another.
// Layers are listed top-down: index 0 is the topmost layer, later indices
// are progressively lower. At least one layer is required.
//
// Writes are always directed at the topmost layer. Reads consult layers in
// order and skip past "does not exist" errors so that lower layers can
// supply paths missing from higher ones; any other error is surfaced
// immediately without consulting further layers.
//
// Globs and directory listings are not merged across layers — the first
// layer whose call succeeds returns its own view. Walks always target the
// top layer.
func NewLayeredFilesystem(layers ...Filesystem) Filesystem {
	if len(layers) == 0 {
		panic("fs: NewLayeredFilesystem requires at least one layer")
	}
	return &layeredFilesystem{layers: layers}
}

type layeredFilesystem struct {
	layers []Filesystem
}

func (f *layeredFilesystem) top() Filesystem { return f.layers[0] }

// readCascade calls op against each layer in order and returns the first
// non-error result. ENOENT errors skip to the next layer; other errors
// return immediately. If every layer reports ENOENT, the last such error
// is returned so callers see the usual [ErrNotExist].
func readCascade[T any](f *layeredFilesystem, op func(Filesystem) (T, error)) (T, error) {
	var zero T
	var last error
	for _, layer := range f.layers {
		v, err := op(layer)
		if err == nil {
			return v, nil
		}
		if !IsNotExist(err) {
			return zero, err
		}
		last = err
	}
	return zero, last
}

func (f *layeredFilesystem) Lstat(name string) (FileInfo, error) {
	return readCascade(f, func(l Filesystem) (FileInfo, error) { return l.Lstat(name) })
}

func (f *layeredFilesystem) Stat(name string) (FileInfo, error) {
	return readCascade(f, func(l Filesystem) (FileInfo, error) { return l.Stat(name) })
}

func (f *layeredFilesystem) DirNames(name string) ([]string, error) {
	return readCascade(f, func(l Filesystem) ([]string, error) { return l.DirNames(name) })
}

func (f *layeredFilesystem) Open(name string) (File, error) {
	return readCascade(f, func(l Filesystem) (File, error) { return l.Open(name) })
}

func (f *layeredFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	// Split write-mode opens (which target the top layer unconditionally)
	// from read-only opens (which cascade through the stack).
	const writeFlags = OptWriteOnly | OptReadWrite | OptCreate | OptTruncate | OptAppend | OptExclusive
	if flags&writeFlags != 0 {
		return f.top().OpenFile(name, flags, mode)
	}
	return readCascade(f, func(l Filesystem) (File, error) { return l.OpenFile(name, flags, mode) })
}

func (f *layeredFilesystem) ReadSymlink(name string) (string, error) {
	return readCascade(f, func(l Filesystem) (string, error) { return l.ReadSymlink(name) })
}

func (f *layeredFilesystem) Glob(pattern string) ([]string, error) {
	return readCascade(f, func(l Filesystem) ([]string, error) { return l.Glob(pattern) })
}

func (f *layeredFilesystem) Usage(name string) (Usage, error) {
	return readCascade(f, func(l Filesystem) (Usage, error) { return l.Usage(name) })
}

func (f *layeredFilesystem) GetXattr(name string, xf XattrFilter) ([]protocol.Xattr, error) {
	return readCascade(f, func(l Filesystem) ([]protocol.Xattr, error) { return l.GetXattr(name, xf) })
}

func (f *layeredFilesystem) PlatformData(name string, withOwnership, withXattrs bool, xf XattrFilter) (protocol.PlatformData, error) {
	return readCascade(f, func(l Filesystem) (protocol.PlatformData, error) {
		return l.PlatformData(name, withOwnership, withXattrs, xf)
	})
}

func (f *layeredFilesystem) Walk(name string, walkFn WalkFunc) error {
	return f.top().Walk(name, walkFn)
}

func (f *layeredFilesystem) Roots() ([]string, error) {
	return f.top().Roots()
}

func (f *layeredFilesystem) Chmod(name string, mode FileMode) error {
	return f.top().Chmod(name, mode)
}

func (f *layeredFilesystem) Lchown(name, uid, gid string) error {
	return f.top().Lchown(name, uid, gid)
}

func (f *layeredFilesystem) Chtimes(name string, atime, mtime time.Time) error {
	return f.top().Chtimes(name, atime, mtime)
}

func (f *layeredFilesystem) Create(name string) (File, error) {
	return f.top().Create(name)
}

func (f *layeredFilesystem) CreateSymlink(target, name string) error {
	return f.top().CreateSymlink(target, name)
}

func (f *layeredFilesystem) Mkdir(name string, perm FileMode) error {
	return f.top().Mkdir(name, perm)
}

func (f *layeredFilesystem) MkdirAll(name string, perm FileMode) error {
	return f.top().MkdirAll(name, perm)
}

func (f *layeredFilesystem) Remove(name string) error {
	return f.top().Remove(name)
}

func (f *layeredFilesystem) RemoveAll(name string) error {
	return f.top().RemoveAll(name)
}

func (f *layeredFilesystem) Rename(oldname, newname string) error {
	return f.top().Rename(oldname, newname)
}

func (f *layeredFilesystem) Hide(name string) error {
	return f.top().Hide(name)
}

func (f *layeredFilesystem) Unhide(name string) error {
	return f.top().Unhide(name)
}

func (f *layeredFilesystem) SetXattr(name string, xattrs []protocol.Xattr, xf XattrFilter) error {
	return f.top().SetXattr(name, xattrs, xf)
}

func (f *layeredFilesystem) SymlinksSupported() bool {
	return f.top().SymlinksSupported()
}

func (f *layeredFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	return f.top().Watch(path, ignore, ctx, ignorePerms)
}

func (f *layeredFilesystem) SameFile(a, b FileInfo) bool {
	return f.top().SameFile(a, b)
}

func (*layeredFilesystem) Type() FilesystemType {
	return FilesystemTypeLayered
}

func (f *layeredFilesystem) URI() string {
	return f.top().URI()
}

func (f *layeredFilesystem) Options() []Option {
	return f.top().Options()
}

func (f *layeredFilesystem) underlying() (Filesystem, bool) {
	return f.top(), true
}
