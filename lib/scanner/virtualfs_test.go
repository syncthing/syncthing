// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

type infiniteFS struct {
	fs.Filesystem
	width    int   // number of files and directories per level
	depth    int   // number of tree levels to simulate
	filesize int64 // size of each file in bytes
}

var errNotSupp = errors.New("not supported")

func (i infiniteFS) Lstat(name string) (fs.FileInfo, error) {
	return fakeInfo{name, i.filesize}, nil
}

func (i infiniteFS) Stat(name string) (fs.FileInfo, error) {
	return fakeInfo{name, i.filesize}, nil
}

func (i infiniteFS) DirNames(name string) ([]string, error) {
	// Returns a list of fake files and directories. Names are such that
	// files appear before directories - this makes it so the scanner will
	// actually see a few files without having to reach the max depth.
	var names []string
	for j := 0; j < i.width; j++ {
		names = append(names, fmt.Sprintf("aa-file-%d", j))
	}
	if len(fs.PathComponents(name)) < i.depth {
		for j := 0; j < i.width; j++ {
			names = append(names, fmt.Sprintf("zz-dir-%d", j))
		}
	}
	return names, nil
}

func (i infiniteFS) ReadDir(name string) ([]stdfs.DirEntry, error) {
	names, err := i.DirNames(name)
	if err != nil {
		return nil, err
	}
	entries := make([]stdfs.DirEntry, 0, len(names))
	for _, n := range names {
		// Construct info for the child.
		childPath := filepath.Join(name, n)
		fi, _ := i.Lstat(childPath)
		entries = append(entries, virtualDirEntry{fi})
	}
	return entries, nil
}

func (i infiniteFS) Open(name string) (fs.File, error) {
	return &fakeFile{name, i.filesize, 0}, nil
}

func (infiniteFS) PlatformData(_ string, _, _ bool, _ fs.XattrFilter) (protocol.PlatformData, error) {
	return protocol.PlatformData{}, nil
}

type singleFileFS struct {
	fs.Filesystem
	name     string
	filesize int64
}

func (s singleFileFS) Lstat(name string) (fs.FileInfo, error) {
	switch name {
	case ".":
		return fakeInfo{".", 0}, nil
	case s.name:
		return fakeInfo{s.name, s.filesize}, nil
	default:
		return nil, errors.New("no such file")
	}
}

func (s singleFileFS) Stat(name string) (fs.FileInfo, error) {
	switch name {
	case ".":
		return fakeInfo{".", 0}, nil
	case s.name:
		return fakeInfo{s.name, s.filesize}, nil
	default:
		return nil, errors.New("no such file")
	}
}

func (s singleFileFS) DirNames(name string) ([]string, error) {
	if name != "." {
		return nil, errors.New("no such file")
	}
	return []string{s.name}, nil
}

func (s singleFileFS) ReadDir(name string) ([]stdfs.DirEntry, error) {
	names, err := s.DirNames(name)
	if err != nil {
		return nil, err
	}
	entries := make([]stdfs.DirEntry, 0, len(names))
	for _, n := range names {
		// For singleFileFS, children of "." is s.name
		childPath := n
		fi, err := s.Lstat(childPath)
		if err != nil {
			return nil, err
		}
		entries = append(entries, virtualDirEntry{fi})
	}
	return entries, nil
}

func (s singleFileFS) Open(name string) (fs.File, error) {
	if name != s.name {
		return nil, errors.New("no such file")
	}
	return &fakeFile{s.name, s.filesize, 0}, nil
}

func (singleFileFS) Options() []fs.Option {
	return nil
}

func (singleFileFS) PlatformData(_ string, _, _ bool, _ fs.XattrFilter) (protocol.PlatformData, error) {
	return protocol.PlatformData{}, nil
}

type virtualDirEntry struct {
	fs.FileInfo
}

func (e virtualDirEntry) Name() string { return e.FileInfo.Name() }
func (e virtualDirEntry) IsDir() bool  { return e.FileInfo.IsDir() }
func (e virtualDirEntry) Type() stdfs.FileMode {
	return stdfs.FileMode(e.FileInfo.Mode()) & stdfs.ModeType
}
func (e virtualDirEntry) Info() (stdfs.FileInfo, error) {
	return virtualStdFileInfo{e.FileInfo}, nil
}

type virtualStdFileInfo struct {
	fs.FileInfo
}

func (i virtualStdFileInfo) Mode() stdfs.FileMode {
	return stdfs.FileMode(i.FileInfo.Mode())
}

type fakeInfo struct {
	name string
	size int64
}

func (f fakeInfo) Name() string     { return f.name }
func (fakeInfo) Mode() fs.FileMode  { return 0o755 }
func (f fakeInfo) Size() int64      { return f.size }
func (fakeInfo) ModTime() time.Time { return time.Unix(1234567890, 0) }
func (f fakeInfo) IsDir() bool {
	return strings.Contains(filepath.Base(f.name), "dir") || f.name == "."
}
func (f fakeInfo) IsRegular() bool          { return !f.IsDir() }
func (fakeInfo) IsSymlink() bool            { return false }
func (fakeInfo) Owner() int                 { return 0 }
func (fakeInfo) Group() int                 { return 0 }
func (fakeInfo) Sys() interface{}           { return nil }
func (fakeInfo) InodeChangeTime() time.Time { return time.Time{} }

type fakeFile struct {
	name       string
	size       int64
	readOffset int64
}

func (f *fakeFile) Name() string {
	return f.name
}

func (f *fakeFile) Read(bs []byte) (int, error) {
	remaining := f.size - f.readOffset
	if remaining == 0 {
		return 0, io.EOF
	}
	if remaining < int64(len(bs)) {
		f.readOffset = f.size
		return int(remaining), nil
	}
	f.readOffset += int64(len(bs))
	return len(bs), nil
}

func (f *fakeFile) Stat() (fs.FileInfo, error) {
	return fakeInfo{f.name, f.size}, nil
}

func (*fakeFile) Write([]byte) (int, error)          { return 0, errNotSupp }
func (*fakeFile) WriteAt([]byte, int64) (int, error) { return 0, errNotSupp }
func (*fakeFile) Close() error                       { return nil }
func (*fakeFile) Truncate(_ int64) error             { return errNotSupp }
func (*fakeFile) ReadAt([]byte, int64) (int, error)  { return 0, errNotSupp }
func (*fakeFile) Seek(int64, int) (int64, error)     { return 0, errNotSupp }
func (*fakeFile) Sync() error                        { return nil }
