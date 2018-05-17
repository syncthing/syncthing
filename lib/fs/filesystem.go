// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The Filesystem interface abstracts access to the file system.
type Filesystem interface {
	Chmod(name string, mode FileMode) error
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
	Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, error)
	Hide(name string) error
	Unhide(name string) error
	Glob(pattern string) ([]string, error)
	Roots() ([]string, error)
	Usage(name string) (Usage, error)
	Type() FilesystemType
	URI() string
	SameFile(fi1, fi2 FileInfo) bool
}

// The File interface abstracts access to a regular file, being a somewhat
// smaller interface than os.File
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt
	Name() string
	Truncate(size int64) error
	Stat() (FileInfo, error)
	Sync() error
}

// The FileInfo interface is almost the same as os.FileInfo, but with the
// Sys method removed (as we don't want to expose whatever is underlying)
// and with a couple of convenience methods added.
type FileInfo interface {
	// Standard things present in os.FileInfo
	Name() string
	Mode() FileMode
	Size() int64
	ModTime() time.Time
	IsDir() bool
	// Extensions
	IsRegular() bool
	IsSymlink() bool
}

// FileMode is similar to os.FileMode
type FileMode uint32

// Usage represents filesystem space usage
type Usage struct {
	Free  int64
	Total int64
}

type Matcher interface {
	ShouldIgnore(name string) bool
	SkipIgnoredDirs() bool
}

type MatchResult interface {
	IsIgnored() bool
}

type Event struct {
	Name string
	Type EventType
}

type EventType int

const (
	NonRemove EventType = 1 + iota
	Remove
	Mixed // Should probably not be necessary to be used in filesystem interface implementation
)

// Merge returns Mixed, except if evType and other are the same and not Mixed.
func (evType EventType) Merge(other EventType) EventType {
	return evType | other
}

func (evType EventType) String() string {
	switch {
	case evType == NonRemove:
		return "non-remove"
	case evType == Remove:
		return "remove"
	case evType == Mixed:
		return "mixed"
	default:
		panic("bug: Unknown event type")
	}
}

var ErrWatchNotSupported = errors.New("watching is not supported")

// Equivalents from os package.

const ModePerm = FileMode(os.ModePerm)
const ModeSetgid = FileMode(os.ModeSetgid)
const ModeSetuid = FileMode(os.ModeSetuid)
const ModeSticky = FileMode(os.ModeSticky)
const ModeSymlink = FileMode(os.ModeSymlink)
const ModeType = FileMode(os.ModeType)
const PathSeparator = os.PathSeparator
const OptAppend = os.O_APPEND
const OptCreate = os.O_CREATE
const OptExclusive = os.O_EXCL
const OptReadOnly = os.O_RDONLY
const OptReadWrite = os.O_RDWR
const OptSync = os.O_SYNC
const OptTruncate = os.O_TRUNC
const OptWriteOnly = os.O_WRONLY

// SkipDir is used as a return value from WalkFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir = filepath.SkipDir

// IsExist is the equivalent of os.IsExist
var IsExist = os.IsExist

// IsNotExist is the equivalent of os.IsNotExist
var IsNotExist = os.IsNotExist

// IsPermission is the equivalent of os.IsPermission
var IsPermission = os.IsPermission

// IsPathSeparator is the equivalent of os.IsPathSeparator
var IsPathSeparator = os.IsPathSeparator

func NewFilesystem(fsType FilesystemType, uri string) Filesystem {
	var fs Filesystem
	switch fsType {
	case FilesystemTypeBasic:
		fs = newBasicFilesystem(uri)
	default:
		l.Debugln("Unknown filesystem", fsType, uri)
		fs = &errorFilesystem{
			fsType: fsType,
			uri:    uri,
			err:    errors.New("filesystem with type " + fsType.String() + " does not exist."),
		}
	}

	if l.ShouldDebug("walkfs") {
		return NewWalkFilesystem(&logFilesystem{fs})
	}

	if l.ShouldDebug("fs") {
		return &logFilesystem{NewWalkFilesystem(fs)}
	}

	return NewWalkFilesystem(fs)
}

// IsInternal returns true if the file, as a path relative to the folder
// root, represents an internal file that should always be ignored. The file
// path must be clean (i.e., in canonical shortest form).
func IsInternal(file string) bool {
	// fs cannot import config, so we hard code .stfolder here (config.DefaultMarkerName)
	internals := []string{".stfolder", ".stignore", ".stversions"}
	pathSep := string(PathSeparator)
	for _, internal := range internals {
		if file == internal {
			return true
		}
		if strings.HasPrefix(file, internal+pathSep) {
			return true
		}
	}
	return false
}

// Canonicalize checks that the file path is valid and returns it in the "canonical" form:
// - /foo/bar -> foo/bar
// - / -> "."
func Canonicalize(file string) (string, error) {
	pathSep := string(PathSeparator)

	if strings.HasPrefix(file, pathSep+pathSep) {
		// The relative path may pretend to be an absolute path within
		// the root, but the double path separator on Windows implies
		// something else and is out of spec.
		return "", ErrNotRelative
	}

	// The relative path should be clean from internal dotdots and similar
	// funkyness.
	file = filepath.Clean(file)

	// It is not acceptable to attempt to traverse upwards.
	switch file {
	case "..":
		return "", ErrNotRelative
	}
	if strings.HasPrefix(file, ".."+pathSep) {
		return "", ErrNotRelative
	}

	if strings.HasPrefix(file, pathSep) {
		if file == pathSep {
			return ".", nil
		}
		return file[1:], nil
	}

	return file, nil
}
